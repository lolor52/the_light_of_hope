package tickers_filling

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"invest_intraday/internal/a_submodule/moex"
	"invest_intraday/internal/a_technical/config"
	"invest_intraday/internal/a_technical/db"
	"invest_intraday/models"

	_ "github.com/lib/pq"
)

// Service отвечает за заполнение таблицы ticker историческими данными.
type Service struct {
	cfg         config.Config
	repo        *db.TickerRepository
	db          *sql.DB
	moexClient  *moex.Client
	securityMap map[string]moex.SecurityInfo
}

// NewService настраивает соединения и возвращает готовый сервис.
func NewService(ctx context.Context, cfg config.Config) (*Service, error) {
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is empty in config")
	}

	dbConn, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	dbConn.SetConnMaxLifetime(10 * time.Minute)
	dbConn.SetMaxOpenConns(5)
	dbConn.SetMaxIdleConns(5)

	if err := dbConn.PingContext(ctx); err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	moexClient, err := moex.NewClient(ctx, cfg.MOEXPassport.Login, cfg.MOEXPassport.Password)
	if err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("create moex client: %w", err)
	}

	service := &Service{
		cfg:         cfg,
		repo:        db.NewTickerRepository(dbConn),
		db:          dbConn,
		moexClient:  moexClient,
		securityMap: make(map[string]moex.SecurityInfo),
	}

	return service, nil
}

// Close освобождает ресурсы сервиса.
func (s *Service) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Run запускает процесс заполнения таблицы ticker.
func (s *Service) Run(ctx context.Context) error {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return fmt.Errorf("load moscow location: %w", err)
	}

	today := time.Now().In(loc)
	startDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)

	for _, tickerCfg := range s.cfg.MOEXTickers {
		if err := s.processTicker(ctx, tickerCfg, startDate); err != nil {
			log.Printf("ticker %s: %v", tickerCfg.Ticker, err)
		}
	}

	return nil
}

func (s *Service) processTicker(ctx context.Context, tickerCfg config.MOEXTicker, startDate time.Time) error {
	securityInfo, err := s.securityInfo(ctx, tickerCfg)
	if err != nil {
		return fmt.Errorf("security info: %w", err)
	}

	date := startDate
	tradingSessionsFound := 0
	sessionsTarget := s.cfg.TickersFillingSessions

	for tradingSessionsFound < sessionsTarget {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		historyRow, err := s.moexClient.GetHistoryRow(ctx, tickerCfg.BoardID, tickerCfg.SecID, date)
		if err != nil {
			return fmt.Errorf("get history row: %w", err)
		}

		sessionActive := historyRow != nil && historyRow.Volume > 0

		_, err = s.repo.GetByDateAndName(ctx, tickerCfg.Ticker, date)
		if err == nil {
			if sessionActive {
				tradingSessionsFound++
			}
			// запись уже есть, перейдём к следующей дате
			date = date.AddDate(0, 0, -1)
			continue
		}
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}

		if !sessionActive {
			if errors.Is(err, db.ErrNotFound) {
				entity := models.Ticker{
					TradingSessionDate:   date,
					TradingSessionActive: false,
					TickerName:           tickerCfg.Ticker,
					SecID:                tickerCfg.SecID,
					BoardID:              tickerCfg.BoardID,
				}
				if err := s.repo.Insert(ctx, entity); err != nil {
					return err
				}
			}
			date = date.AddDate(0, 0, -1)
			continue
		}

		tradingSessionsFound++

		metrics, err := s.collectMetrics(ctx, tickerCfg, securityInfo, *historyRow)
		if err != nil {
			log.Printf("metrics for %s %s: %v", tickerCfg.Ticker, date.Format("2006-01-02"), err)
		}

		entity := models.Ticker{
			TradingSessionDate:   date,
			TradingSessionActive: true,
			TickerName:           tickerCfg.Ticker,
			SecID:                tickerCfg.SecID,
			BoardID:              tickerCfg.BoardID,
			VWAP:                 metrics.VWAP,
			VAL:                  metrics.VAL,
			VAH:                  metrics.VAH,
			FlatTrendFilter:      metrics.FlatTrendFilter,
			Volatility:           metrics.Volatility,
			Liquidity:            metrics.Liquidity,
		}

		if err := s.repo.Insert(ctx, entity); err != nil {
			return err
		}

		date = date.AddDate(0, 0, -1)
	}

	return nil
}

type sessionMetrics struct {
	VWAP            *string
	VAL             *string
	VAH             *string
	FlatTrendFilter *string
	Volatility      *string
	Liquidity       *string
}

func (s *Service) collectMetrics(ctx context.Context, tickerCfg config.MOEXTicker, securityInfo moex.SecurityInfo, historyRow moex.HistoryRow) (sessionMetrics, error) {
	metrics := sessionMetrics{}

	candles, err := s.moexClient.GetMinuteCandles(ctx, tickerCfg.BoardID, tickerCfg.SecID, historyRow.TradeDate)
	if err != nil {
		return metrics, fmt.Errorf("minute candles: %w", err)
	}

	vwapValue, err := calculateVWAP(candles)
	if err == nil {
		metrics.VWAP = formatFloat(vwapValue)
	}

	valueArea, err := calculateValueArea(candles, securityInfo.MinStep)
	if err == nil {
		metrics.VAL = formatFloat(valueArea.VAL)
		metrics.VAH = formatFloat(valueArea.VAH)
	}

	prevRow, err := s.prevTradingDay(ctx, tickerCfg, historyRow.TradeDate)
	if err == nil && prevRow != nil {
		filterValue, err := calculateFlatTrendFilter(candles, historyRow, *prevRow)
		if err == nil {
			metrics.FlatTrendFilter = formatFloat(filterValue)
		}
	}

	historyWindow, err := s.moexClient.GetHistoryWindow(ctx, tickerCfg.BoardID, tickerCfg.SecID, historyRow.TradeDate.AddDate(0, 0, -60), historyRow.TradeDate)
	if err == nil && len(historyWindow) > 0 {
		volValue, err := calculateVolatility(historyRow, historyWindow)
		if err == nil {
			metrics.Volatility = formatFloat(volValue)
		}
	}

	marketData, err := s.moexClient.GetMarketData(ctx, tickerCfg.BoardID, tickerCfg.SecID, historyRow.TradeDate)
	if err == nil {
		orderBook, err := s.moexClient.GetOrderBook(ctx, tickerCfg.BoardID, tickerCfg.SecID, historyRow.TradeDate, 5)
		if err == nil {
			liquidityValue, err := calculateLiquidity(candles, marketData, orderBook, securityInfo)
			if err == nil {
				metrics.Liquidity = formatFloat(liquidityValue)
			}
		}
	}

	return metrics, nil
}

func (s *Service) securityInfo(ctx context.Context, tickerCfg config.MOEXTicker) (moex.SecurityInfo, error) {
	key := tickerCfg.Ticker
	if info, ok := s.securityMap[key]; ok {
		return info, nil
	}
	info, err := s.moexClient.GetSecurityInfo(ctx, tickerCfg.BoardID, tickerCfg.SecID)
	if err != nil {
		return moex.SecurityInfo{}, err
	}
	s.securityMap[key] = info
	return info, nil
}

func (s *Service) prevTradingDay(ctx context.Context, tickerCfg config.MOEXTicker, from time.Time) (*moex.HistoryRow, error) {
	date := from.AddDate(0, 0, -1)
	for i := 0; i < 10; i++ {
		row, err := s.moexClient.GetHistoryRow(ctx, tickerCfg.BoardID, tickerCfg.SecID, date)
		if err != nil {
			return nil, err
		}
		if row != nil && row.Volume > 0 {
			return row, nil
		}
		date = date.AddDate(0, 0, -1)
	}
	return nil, fmt.Errorf("previous trading day not found for %s", tickerCfg.Ticker)
}

func formatFloat(value float64) *string {
	formatted := fmt.Sprintf("%.6f", value)
	return &formatted
}
