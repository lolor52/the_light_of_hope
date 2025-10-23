package tickers_filling

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"invest_intraday/internal/a_submodule/indicators"
	"invest_intraday/internal/a_technical/db"
	"invest_intraday/models"
)

// Config описывает параметры заполнения исторических данных.
type Config struct {
	// SessionsTarget определяет количество активных торговых сессий, которые нужно покрыть.
	SessionsTarget int
	// MaxInactiveDays ограничивает общее количество проверяемых календарных дней.
	MaxInactiveDays int
}

// Service отвечает за заполнение таблицы ticker_history историческими данными.
type Service struct {
	tickerInfoRepo *db.TickerInfoRepository
	historyRepo    *db.TickerRepository
	valueAreaCalc  *indicators.Calculator
	config         Config
	now            func() time.Time
	moscowLoc      *time.Location
}

// FillStats описывает статистику выполнения заполнения истории тикеров.
type FillStats struct {
	// ExistingRecords содержит количество уже существовавших записей ticker_history.
	ExistingRecords int
	// CreatedRecords содержит количество новых записей ticker_history, добавленных за выполнение.
	CreatedRecords int
	// ActiveSessionDates содержит количество дат, в которые была активная основная торговая сессия.
	ActiveSessionDates int
	// InactiveSessionDates содержит количество дат без активной основной торговой сессии.
	InactiveSessionDates int
}

func (s *FillStats) add(other FillStats) {
	s.ExistingRecords += other.ExistingRecords
	s.CreatedRecords += other.CreatedRecords
	s.ActiveSessionDates += other.ActiveSessionDates
	s.InactiveSessionDates += other.InactiveSessionDates
}

// NewService создаёт сервис заполнения исторических данных по тикерам.
func NewService(
	tickerInfoRepo *db.TickerInfoRepository,
	historyRepo *db.TickerRepository,
	valueAreaCalc *indicators.Calculator,
	cfg Config,
) (*Service, error) {
	if tickerInfoRepo == nil || historyRepo == nil || valueAreaCalc == nil {
		return nil, fmt.Errorf("tickers filling: missing dependencies")
	}

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*60*60)
	}

	service := &Service{
		tickerInfoRepo: tickerInfoRepo,
		historyRepo:    historyRepo,
		valueAreaCalc:  valueAreaCalc,
		config:         cfg,
		now:            time.Now,
		moscowLoc:      loc,
	}

	if service.config.SessionsTarget < 0 {
		service.config.SessionsTarget = 0
	}
	if service.config.MaxInactiveDays < 0 {
		service.config.MaxInactiveDays = 0
	}

	return service, nil
}

// WithNow позволяет подменить источник текущего времени (используется в тестах).
func (s *Service) WithNow(now func() time.Time) {
	if now == nil {
		return
	}
	s.now = now
}

// Fill выполняет заполнение данных для всех тикеров из справочника и возвращает агрегированную статистику.
func (s *Service) Fill(ctx context.Context) (FillStats, error) {
	var total FillStats

	if s.config.SessionsTarget == 0 || s.config.MaxInactiveDays == 0 {
		return total, nil
	}

	tickers, err := s.tickerInfoRepo.ListAll(ctx)
	if err != nil {
		return FillStats{}, fmt.Errorf("tickers filling: list tickers: %w", err)
	}

	startDate := s.yesterdayMoscow()

	for _, ticker := range tickers {
		stats, err := s.fillTicker(ctx, ticker, startDate)
		if err != nil {
			return FillStats{}, fmt.Errorf("tickers filling: %s: %w", ticker.TickerName, err)
		}
		total.add(stats)
	}

	return total, nil
}

func (s *Service) fillTicker(ctx context.Context, ticker models.TickerInfo, startDate time.Time) (FillStats, error) {
	var stats FillStats
	activeSessions := 0
	iterations := 0
	sessionDate := startDate

	for iterations < s.config.MaxInactiveDays && activeSessions < s.config.SessionsTarget {
		if err := ctx.Err(); err != nil {
			return stats, err
		}

		dbDate := time.Date(sessionDate.Year(), sessionDate.Month(), sessionDate.Day(), 0, 0, 0, 0, time.UTC)

		history, err := s.historyRepo.GetByDateAndName(ctx, ticker.TickerName, dbDate)
		if err == nil {
			stats.ExistingRecords++
			if history.TradingSessionActive {
				activeSessions++
				stats.ActiveSessionDates++
			} else {
				stats.InactiveSessionDates++
			}
		} else if errors.Is(err, db.ErrNotFound) {
			sessionData, calcErr := s.computeSession(ctx, ticker.ID, sessionDate)
			if calcErr != nil {
				return stats, calcErr
			}

			if sessionData.active {
				activeSessions++
				stats.ActiveSessionDates++
			} else {
				stats.InactiveSessionDates++
			}

			entity := models.TickerHistory{
				TradingSessionDate:   dbDate,
				TradingSessionActive: sessionData.active,
				TickerInfoID:         ticker.ID,
				VWAP:                 sessionData.vwap,
				VAL:                  sessionData.val,
				VAH:                  sessionData.vah,
				SwingCountPaired:     nil,
			}

			if err := s.historyRepo.Insert(ctx, entity); err != nil {
				return stats, fmt.Errorf("insert ticker_history: %w", err)
			}
			stats.CreatedRecords++
		} else {
			return stats, fmt.Errorf("get ticker_history: %w", err)
		}

		iterations++
		sessionDate = sessionDate.AddDate(0, 0, -1)

		if iterations < s.config.MaxInactiveDays && activeSessions < s.config.SessionsTarget {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return stats, ctx.Err()
			}
		}
	}

	return stats, nil
}

type sessionComputation struct {
	active bool
	vwap   *string
	val    *string
	vah    *string
}

func (s *Service) computeSession(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (sessionComputation, error) {
	var result sessionComputation

	valueArea, err := s.valueAreaCalc.Calculate(ctx, tickerInfoID, sessionDate)
	if err != nil {
		if errors.Is(err, indicators.ErrNoTrades) {
			return result, nil
		}
		return result, fmt.Errorf("calculate value area: %w", err)
	}

	result.active = true
	result.vwap = stringPointer(strconv.FormatFloat(valueArea.VWAP, 'f', 6, 64))
	result.val = stringPointer(strconv.FormatFloat(valueArea.VAL, 'f', 6, 64))
	result.vah = stringPointer(strconv.FormatFloat(valueArea.VAH, 'f', 6, 64))

	return result, nil
}

func (s *Service) yesterdayMoscow() time.Time {
	now := s.now().In(s.moscowLoc)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.moscowLoc)
	return midnight.AddDate(0, 0, -1)
}

func stringPointer(value string) *string {
	return &value
}
