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
	swingCalc      *indicators.SwingCountPairedCalculator
	config         Config
	now            func() time.Time
	moscowLoc      *time.Location
}

// NewService создаёт сервис заполнения исторических данных по тикерам.
func NewService(
	tickerInfoRepo *db.TickerInfoRepository,
	historyRepo *db.TickerRepository,
	valueAreaCalc *indicators.Calculator,
	swingCalc *indicators.SwingCountPairedCalculator,
	cfg Config,
) (*Service, error) {
	if tickerInfoRepo == nil || historyRepo == nil || valueAreaCalc == nil || swingCalc == nil {
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
		swingCalc:      swingCalc,
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

// Fill выполняет заполнение данных для всех тикеров из справочника.
func (s *Service) Fill(ctx context.Context) error {
	if s.config.SessionsTarget == 0 || s.config.MaxInactiveDays == 0 {
		return nil
	}

	tickers, err := s.tickerInfoRepo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("tickers filling: list tickers: %w", err)
	}

	startDate := s.yesterdayMoscow()

	for _, ticker := range tickers {
		if err := s.fillTicker(ctx, ticker, startDate); err != nil {
			return fmt.Errorf("tickers filling: %s: %w", ticker.TickerName, err)
		}
	}

	return nil
}

func (s *Service) fillTicker(ctx context.Context, ticker models.TickerInfo, startDate time.Time) error {
	activeSessions := 0
	iterations := 0
	sessionDate := startDate

	for iterations < s.config.MaxInactiveDays && activeSessions < s.config.SessionsTarget {
		if err := ctx.Err(); err != nil {
			return err
		}

		dbDate := time.Date(sessionDate.Year(), sessionDate.Month(), sessionDate.Day(), 0, 0, 0, 0, time.UTC)

		history, err := s.historyRepo.GetByDateAndName(ctx, ticker.TickerName, dbDate)
		if err == nil {
			if history.TradingSessionActive {
				activeSessions++
			}
		} else if errors.Is(err, db.ErrNotFound) {
			sessionData, calcErr := s.computeSession(ctx, ticker.ID, sessionDate)
			if calcErr != nil {
				return calcErr
			}

			if sessionData.active {
				activeSessions++
			}

			entity := models.TickerHistory{
				TradingSessionDate:   dbDate,
				TradingSessionActive: sessionData.active,
				TickerInfoID:         ticker.ID,
				VWAP:                 sessionData.vwap,
				VAL:                  sessionData.val,
				VAH:                  sessionData.vah,
				SwingCountPaired:     sessionData.swing,
			}

			if err := s.historyRepo.Insert(ctx, entity); err != nil {
				return fmt.Errorf("insert ticker_history: %w", err)
			}
		} else {
			return fmt.Errorf("get ticker_history: %w", err)
		}

		iterations++
		sessionDate = sessionDate.AddDate(0, 0, -1)
	}

	return nil
}

type sessionComputation struct {
	active bool
	vwap   *string
	val    *string
	vah    *string
	swing  *string
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

	swingCount, err := s.swingCalc.Calculate(ctx, tickerInfoID, sessionDate)
	if err != nil {
		if errors.Is(err, indicators.ErrNoTrades) {
			return result, nil
		}
		return result, fmt.Errorf("calculate swing count paired: %w", err)
	}

	result.active = true
	result.vwap = stringPointer(strconv.FormatFloat(valueArea.VWAP, 'f', 6, 64))
	result.val = stringPointer(strconv.FormatFloat(valueArea.VAL, 'f', 6, 64))
	result.vah = stringPointer(strconv.FormatFloat(valueArea.VAH, 'f', 6, 64))
	result.swing = stringPointer(strconv.Itoa(swingCount))

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
