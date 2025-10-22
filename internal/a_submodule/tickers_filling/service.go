package tickers_filling

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"invest_intraday/internal/a_submodule/indicators"
	"invest_intraday/internal/a_submodule/moex"
	"invest_intraday/internal/a_technical/config"
	"invest_intraday/internal/a_technical/db"
	"invest_intraday/models"

	_ "github.com/lib/pq"
)

// Service отвечает за заполнение таблицы ticker_history историческими данными.
type Service struct {
	cfg         config.Config
	repo        *db.TickerRepository
	tickerInfos []models.TickerInfo
	db          *sql.DB
	moexClient  *moex.Client
	securityMap map[string]moex.SecurityInfo
}

// RunStats содержит статистику выполнения сервиса.
type RunStats struct {
	Existing int
	Created  int
}

// flatTrendBandWidth задаёт параметр b из docs/flat_trend_filter.md.
const flatTrendBandWidth = 0.05

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

	tickerInfoRepo := db.NewTickerInfoRepository(dbConn)
	tickerInfos, err := tickerInfoRepo.ListAll(ctx)
	if err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("load ticker info: %w", err)
	}

	service := &Service{
		cfg:         cfg,
		repo:        db.NewTickerRepository(dbConn),
		tickerInfos: tickerInfos,
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

// Run запускает процесс заполнения таблицы ticker_history.
func (s *Service) Run(ctx context.Context) (RunStats, error) {
	var stats RunStats
	var pending []pendingRecord

	log.Printf("tickers_filling: запуск модуля")

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return stats, fmt.Errorf("load moscow location: %w", err)
	}

	today := time.Now().In(loc)
	startDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)

	for _, tickerInfo := range s.tickerInfos {
		tickerStats, tickerPending, err := s.processTicker(ctx, tickerInfo, startDate)
		stats.Existing += tickerStats.Existing
		stats.Created += tickerStats.Created
		if err != nil {
			log.Printf("tickers_filling: ошибка при обработке тикера %s: %v", tickerInfo.TickerName, err)
			continue
		}
		pending = append(pending, tickerPending...)
	}

	if len(pending) > 0 {
		created, err := s.finalizeAndInsertPending(ctx, pending)
		if err != nil {
			return stats, err
		}
		stats.Created += created
	}

	return stats, nil
}

func (s *Service) processTicker(ctx context.Context, tickerCfg models.TickerInfo, startDate time.Time) (RunStats, []pendingRecord, error) {
	var stats RunStats
	var pending []pendingRecord

	securityInfo, err := s.securityInfo(ctx, tickerCfg)
	if err != nil {
		return stats, nil, fmt.Errorf("security info: %w", err)
	}

	date := startDate
	tradingSessionsFound := 0
	sessionsTarget := s.cfg.TickersFillingSessions
	maxInactiveDays := s.cfg.TickersFillingMaxInactiveDays
	consecutiveInactiveDays := 0

	for tradingSessionsFound < sessionsTarget {
		select {
		case <-ctx.Done():
			return stats, nil, ctx.Err()
		default:
		}

		historyEntity, err := s.repo.GetByDateAndName(ctx, tickerCfg.TickerName, date)
		if err == nil {
			stats.Existing++
			if historyEntity.TradingSessionActive {
				tradingSessionsFound++
				consecutiveInactiveDays = 0
			} else if maxInactiveDays > 0 {
				consecutiveInactiveDays++
				if consecutiveInactiveDays >= maxInactiveDays {
					log.Printf("tickers_filling: тикер %s не торговался последние %d дней, остановлено заполнение", tickerCfg.TickerName, maxInactiveDays)
					return stats, pending, nil
				}
			}
			date = date.AddDate(0, 0, -1)
			continue
		}
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return stats, nil, err
		}

		historyRow, err := s.moexClient.GetHistoryRow(ctx, tickerCfg.BoardID, tickerCfg.SecID, date)
		if err != nil {
			return stats, nil, fmt.Errorf("get history row: %w", err)
		}

		sessionActive := historyRow != nil && historyRow.Volume > 0

		if !sessionActive {
			if maxInactiveDays > 0 {
				consecutiveInactiveDays++
				if consecutiveInactiveDays >= maxInactiveDays {
					log.Printf("tickers_filling: тикер %s не торговался последние %d дней, остановлено заполнение", tickerCfg.TickerName, maxInactiveDays)
					return stats, pending, nil
				}
			}

			entity := models.TickerHistory{
				TradingSessionDate:   date,
				TradingSessionActive: false,
				TickerInfoID:         tickerCfg.ID,
			}
			if insertErr := s.repo.Insert(ctx, entity); insertErr != nil {
				return stats, nil, insertErr
			}
			stats.Created++

			date = date.AddDate(0, 0, -1)
			continue
		}

		tradingSessionsFound++
		consecutiveInactiveDays = 0

		metrics, err := s.collectMetrics(ctx, tickerCfg, securityInfo, *historyRow)
		if err != nil {
			log.Printf("tickers_filling: не удалось рассчитать метрики для %s %s: %v", tickerCfg.TickerName, date.Format("2006-01-02"), err)
		}

		entity := models.TickerHistory{
			TradingSessionDate:   date,
			TradingSessionActive: true,
			TickerInfoID:         tickerCfg.ID,
			VWAP:                 metrics.VWAP,
			VAL:                  metrics.VAL,
			VAH:                  metrics.VAH,
		}

		pending = append(pending, pendingRecord{entity: entity, raw: metrics})

		date = date.AddDate(0, 0, -1)
	}

	return stats, pending, nil
}

type sessionRawMetrics struct {
	VWAP       *string
	VAL        *string
	VAH        *string
	FlatTrend  *float64
	Volatility *float64
	Liquidity  *indicators.LiquidityMetrics
}

type pendingRecord struct {
	entity models.TickerHistory
	raw    sessionRawMetrics
}

func (s *Service) collectMetrics(ctx context.Context, tickerCfg models.TickerInfo, securityInfo moex.SecurityInfo, historyRow moex.HistoryRow) (sessionRawMetrics, error) {
	metrics := sessionRawMetrics{}

	intervals, err := s.moexClient.GetBoardSessions(ctx, tickerCfg.BoardID, historyRow.TradeDate)
	if err != nil {
		return metrics, fmt.Errorf("load sessions: %w", err)
	}
	schedule, err := resolveSessionSchedule(intervals)
	if err != nil {
		return metrics, fmt.Errorf("resolve session schedule: %w", err)
	}

	candles, err := s.moexClient.GetMinuteCandles(ctx, tickerCfg.BoardID, tickerCfg.SecID, historyRow.TradeDate)
	if err != nil {
		return metrics, fmt.Errorf("minute candles: %w", err)
	}

	trades, err := s.moexClient.GetTrades(ctx, tickerCfg.BoardID, tickerCfg.SecID, historyRow.TradeDate)
	if err != nil {
		log.Printf("tickers_filling: не удалось загрузить сделки %s: %v", tickerCfg.TickerName, err)
	}
	sessionTrades := filterMainSessionTrades(trades, schedule)

	prevRow, err := s.prevTradingDay(ctx, tickerCfg, historyRow.TradeDate)
	prevClose := 0.0
	if err == nil && prevRow != nil {
		prevClose = prevRow.Close
	}

	fallbacks := priceFallbacks{
		PrevSessionClose: prevClose,
		FirstTradePrice:  firstTradePrice(trades, schedule),
	}
	currentSeries, err := buildMinuteSeries(candles, schedule, fallbacks, securityInfo.LotSize)
	if err != nil {
		return metrics, fmt.Errorf("build minute series: %w", err)
	}

	if vwapValue, err := calculateVWAP(currentSeries); err == nil {
		metrics.VWAP = formatFloat(vwapValue)
	}
	if valueArea, err := calculateValueArea(currentSeries, securityInfo.MinStep); err == nil {
		metrics.VAL = formatFloat(valueArea.VAL)
		metrics.VAH = formatFloat(valueArea.VAH)
	}

	flatParams := indicators.FlatTrendParams{BandWidthFactor: flatTrendBandWidth}
	if score, err := indicators.CalculateFlatTrendScore(currentSeries, flatParams); err == nil {
		metrics.FlatTrend = &score
	}

	if volMetrics, err := indicators.CalculateVolatilityMetrics(sessionTrades); err == nil {
		metrics.Volatility = &volMetrics.Score
	}

	if liqMetrics, err := indicators.CalculateLiquidityMetrics(sessionTrades); err == nil {
		metrics.Liquidity = &liqMetrics
	}

	return metrics, nil
}

func (s *Service) finalizeAndInsertPending(ctx context.Context, pending []pendingRecord) (int, error) {
	grouped := make(map[time.Time][]*pendingRecord)
	for i := range pending {
		record := &pending[i]
		grouped[record.entity.TradingSessionDate] = append(grouped[record.entity.TradingSessionDate], record)
	}

	for _, records := range grouped {
		assignFlatTrendScore(records)
		applyLiquidityScore(records)
		assignVolatilityScore(records)
	}

	created := 0
	for i := range pending {
		if err := s.repo.Insert(ctx, pending[i].entity); err != nil {
			return created, err
		}
		created++
	}

	return created, nil
}

func assignFlatTrendScore(records []*pendingRecord) {
	for _, record := range records {
		if record.raw.FlatTrend == nil {
			continue
		}
		record.entity.FlatTrendFilter = formatFloat(*record.raw.FlatTrend)
	}
}

func applyLiquidityScore(records []*pendingRecord) {
	type item struct {
		rec    *pendingRecord
		metric *indicators.LiquidityMetrics
	}

	var items []item
	for _, record := range records {
		if record.raw.Liquidity == nil {
			continue
		}
		items = append(items, item{rec: record, metric: record.raw.Liquidity})
	}
	if len(items) == 0 {
		return
	}

	values := make([]float64, len(items))
	for i, it := range items {
		values[i] = it.metric.LogLiquidity
	}
	scores := indicators.CrossSectionScore(values)
	for i, it := range items {
		score := scores[i]
		if math.IsNaN(score) {
			continue
		}
		it.rec.entity.Liquidity = formatFloat(score)
	}
}

func assignVolatilityScore(records []*pendingRecord) {
	for _, record := range records {
		if record.raw.Volatility == nil {
			continue
		}
		record.entity.Volatility = formatFloat(*record.raw.Volatility)
	}
}

func (s *Service) securityInfo(ctx context.Context, tickerCfg models.TickerInfo) (moex.SecurityInfo, error) {
	key := tickerCfg.TickerName
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

func (s *Service) prevTradingDay(ctx context.Context, tickerCfg models.TickerInfo, from time.Time) (*moex.HistoryRow, error) {
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
	return nil, fmt.Errorf("previous trading day not found for %s", tickerCfg.TickerName)
}
