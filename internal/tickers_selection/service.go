package tickers_selection

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"invest_intraday/internal/a_submodule/tickers_filling"
	"invest_intraday/internal/a_technical/config"
	"invest_intraday/internal/a_technical/db"
	"invest_intraday/models"

	_ "github.com/lib/pq"
)

const (
	requiredSessions = 5
)

// Regime описывает ожидаемый тип следующей торговой сессии.
type Regime string

const (
	// RegimeTrend соответствует режиму направленного движения.
	RegimeTrend Regime = "Trend"
	// RegimeRange соответствует режиму бокового диапазона.
	RegimeRange Regime = "Range"
)

// Result содержит итоговые оценки по тикеру.
type Result struct {
	Ticker             string
	Regime             Regime
	FinalScore         float64
	MeanReversionScore float64
	MomentumScore      float64
	TrendScore         float64
	DeltaVWAPPct       float64
	OverlapPercent     float64
	Breakout           bool
}

// RunResult описывает итог выполнения сервиса отбора тикеров.
type RunResult struct {
	FillingStats tickers_filling.RunStats
	Selected     []Result
}

// Service инкапсулирует бизнес-логику модуля tickers_selection.
type Service struct {
	cfg  config.Config
	repo *db.TickerRepository
	db   *sql.DB
}

// NewService открывает соединение с БД и подготавливает сервис.
func NewService(ctx context.Context, cfg config.Config) (*Service, error) {
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is empty in config")
	}

	dbConn, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := dbConn.PingContext(ctx); err != nil {
		dbConn.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	service := &Service{
		cfg:  cfg,
		repo: db.NewTickerRepository(dbConn),
		db:   dbConn,
	}

	return service, nil
}

// Close закрывает соединение с БД.
func (s *Service) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Run запускает заполнение данных и рассчитывает лучшие тикеры.
func (s *Service) Run(ctx context.Context) (RunResult, error) {
	var result RunResult

	fillingService, err := tickers_filling.NewService(ctx, s.cfg)
	if err != nil {
		return result, fmt.Errorf("init tickers_filling: %w", err)
	}
	defer fillingService.Close()

	stats, err := fillingService.Run(ctx)
	if err != nil {
		return result, fmt.Errorf("run tickers_filling: %w", err)
	}
	result.FillingStats = stats

	selected, err := s.selectTickers(ctx)
	if err != nil {
		return result, err
	}
	result.Selected = selected

	return result, nil
}

func (s *Service) selectTickers(ctx context.Context) ([]Result, error) {
	selectionCount := s.cfg.TickersSelectionCount
	if selectionCount <= 0 {
		selectionCount = 4
	}

	results := make([]Result, 0, len(s.cfg.MOEXTickers))
	for _, tickerCfg := range s.cfg.MOEXTickers {
		sessions, err := s.repo.ListLastActiveSessions(ctx, tickerCfg.Ticker, requiredSessions)
		if err != nil {
			return nil, fmt.Errorf("load sessions for %s: %w", tickerCfg.Ticker, err)
		}
		if len(sessions) < requiredSessions {
			log.Printf("tickers_selection: недостаточно активных сессий для %s: %d/%d", tickerCfg.Ticker, len(sessions), requiredSessions)
			continue
		}

		metrics, err := buildSessionMetrics(sessions)
		if err != nil {
			log.Printf("tickers_selection: пропуск %s: %v", tickerCfg.Ticker, err)
			continue
		}

		scores, err := calculateScores(metrics)
		if err != nil {
			log.Printf("tickers_selection: ошибка расчёта для %s: %v", tickerCfg.Ticker, err)
			continue
		}

		results = append(results, Result{
			Ticker:             tickerCfg.Ticker,
			Regime:             scores.Regime,
			FinalScore:         scores.FinalScore,
			MeanReversionScore: scores.MeanReversionScore,
			MomentumScore:      scores.MomentumScore,
			TrendScore:         scores.TrendScore,
			DeltaVWAPPct:       scores.DeltaVWAPPct,
			OverlapPercent:     scores.OverlapPercent,
			Breakout:           scores.Breakout,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].FinalScore == results[j].FinalScore {
			return results[i].Ticker < results[j].Ticker
		}
		return results[i].FinalScore > results[j].FinalScore
	})

	if len(results) > selectionCount {
		results = results[:selectionCount]
	}

	if len(results) == 0 {
		log.Printf("tickers_selection: тикеры не выбраны")
		return results, nil
	}

	tickers := make([]string, len(results))
	for i, item := range results {
		tickers[i] = item.Ticker
	}
	log.Printf("tickers_selection: выбраны тикеры: %s", strings.Join(tickers, ", "))

	return results, nil
}

type sessionMetrics struct {
	VAH        float64
	VAL        float64
	VWAP       float64
	Liquidity  float64
	Volatility float64
	FlatTrend  float64
}

func buildSessionMetrics(sessions []models.Ticker) ([]sessionMetrics, error) {
	metrics := make([]sessionMetrics, 0, len(sessions))
	for _, session := range sessions {
		vah, err := parseMetric(session.VAH, "VAH")
		if err != nil {
			return nil, err
		}
		val, err := parseMetric(session.VAL, "VAL")
		if err != nil {
			return nil, err
		}
		vwap, err := parseMetric(session.VWAP, "VWAP")
		if err != nil {
			return nil, err
		}
		liquidity, err := parseMetric(session.Liquidity, "Liquidity")
		if err != nil {
			return nil, err
		}
		volatility, err := parseMetric(session.Volatility, "Volatility")
		if err != nil {
			return nil, err
		}
		flat, err := parseMetric(session.FlatTrendFilter, "FlatTrendFilter")
		if err != nil {
			return nil, err
		}

		metrics = append(metrics, sessionMetrics{
			VAH:        vah,
			VAL:        val,
			VWAP:       vwap,
			Liquidity:  liquidity,
			Volatility: volatility,
			FlatTrend:  flat,
		})
	}
	return metrics, nil
}

func parseMetric(value *string, name string) (float64, error) {
	if value == nil {
		return 0, fmt.Errorf("metric %s is nil", name)
	}
	parsed, err := strconv.ParseFloat(*value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}
