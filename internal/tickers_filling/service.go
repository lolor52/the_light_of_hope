package tickers_filling

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"invest_intraday/internal/a_technical/db"
	"invest_intraday/internal/indicators"
	"invest_intraday/models"
)

const (
	moscowLocationName = "Europe/Moscow"
	boardTQBR          = "TQBR"
)

// tickerInfoLister описывает зависимость, предоставляющую перечень тикеров.
type tickerInfoLister interface {
	ListAll(ctx context.Context) ([]models.TickerInfo, error)
}

// tickerHistoryStorage инкапсулирует операции чтения и записи истории торгов.
type tickerHistoryStorage interface {
	GetByDateAndName(ctx context.Context, name string, sessionDate time.Time) (models.TickerHistory, error)
	Insert(ctx context.Context, entity models.TickerHistory) error
}

// valueAreaCalculator описывает вычислитель VWAP/VAL/VAH основной сессии.
type valueAreaCalculator interface {
	CalculateMainSessionProfile(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (indicators.SessionProfile, error)
}

// Service реализует заполнение таблицы ticker_history на основе данных Alor.
type Service struct {
	tickerInfos   tickerInfoLister
	tickerHistory tickerHistoryStorage
	calculator    valueAreaCalculator

	sessionsLimit int
	maxInactive   int

	nowFunc func() time.Time
	loc     *time.Location
}

// Option позволяет переопределить параметры сервиса.
type Option func(*Service)

// WithNowFunc задаёт функцию получения текущего времени.
func WithNowFunc(fn func() time.Time) Option {
	return func(s *Service) {
		if fn != nil {
			s.nowFunc = fn
		}
	}
}

// WithLocation переопределяет часовую зону для вычисления дат.
func WithLocation(loc *time.Location) Option {
	return func(s *Service) {
		if loc != nil {
			s.loc = loc
		}
	}
}

// NewService конструирует сервис заполнения истории тикеров.
func NewService(
	tickerInfos tickerInfoLister,
	tickerHistory tickerHistoryStorage,
	calculator valueAreaCalculator,
	sessionsLimit int,
	maxInactive int,
	opts ...Option,
) (*Service, error) {
	if tickerInfos == nil {
		return nil, errors.New("tickers_filling: ticker info repository is required")
	}
	if tickerHistory == nil {
		return nil, errors.New("tickers_filling: ticker history repository is required")
	}
	if calculator == nil {
		return nil, errors.New("tickers_filling: value area calculator is required")
	}
	if sessionsLimit <= 0 {
		return nil, errors.New("tickers_filling: sessions limit must be positive")
	}
	if maxInactive <= 0 {
		return nil, errors.New("tickers_filling: max inactive days must be positive")
	}

	loc, err := time.LoadLocation(moscowLocationName)
	if err != nil {
		return nil, fmt.Errorf("tickers_filling: load location: %w", err)
	}

	s := &Service{
		tickerInfos:   tickerInfos,
		tickerHistory: tickerHistory,
		calculator:    calculator,
		sessionsLimit: sessionsLimit,
		maxInactive:   maxInactive,
		nowFunc:       time.Now,
		loc:           loc,
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.loc == nil {
		return nil, errors.New("tickers_filling: location is required")
	}
	if s.nowFunc == nil {
		s.nowFunc = time.Now
	}

	return s, nil
}

// Fill последовательно обходит тикеры и добавляет недостающие записи в историю.
func (s *Service) Fill(ctx context.Context) error {
	if s == nil {
		return errors.New("tickers_filling: nil service")
	}

	tickers, err := s.tickerInfos.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list ticker info: %w", err)
	}

	startDate := s.startDate()

	for _, info := range tickers {
		if !strings.EqualFold(strings.TrimSpace(info.BoardID), boardTQBR) {
			continue
		}

		if err := s.processTicker(ctx, info, startDate); err != nil {
			return fmt.Errorf("process ticker %s: %w", info.TickerName, err)
		}
	}

	return nil
}

func (s *Service) startDate() time.Time {
	now := s.nowFunc().In(s.loc)
	yesterday := now.AddDate(0, 0, -1)
	return time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, s.loc)
}

func (s *Service) processTicker(ctx context.Context, info models.TickerInfo, startDate time.Time) error {
	activeSessions := 0

	for i := 0; i < s.maxInactive && activeSessions < s.sessionsLimit; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		sessionDate := startDate.AddDate(0, 0, -i)

		record, err := s.tickerHistory.GetByDateAndName(ctx, info.TickerName, sessionDate)
		switch {
		case err == nil:
			if record.TradingSessionActive {
				activeSessions++
			}
			continue
		case errors.Is(err, db.ErrNotFound):
			// continue processing
		default:
			return fmt.Errorf("load ticker history for %s: %w", sessionDate.Format(time.DateOnly), err)
		}

		active, profile, err := s.detectSession(ctx, info, sessionDate)
		if err != nil {
			return err
		}

		if active {
			activeSessions++
		}

		entity := models.TickerHistory{
			TradingSessionDate:   sessionDate,
			TradingSessionActive: active,
			TickerInfoID:         info.ID,
			SwingCountPaired:     nil,
		}

		if active && profile != nil {
			vwap := formatFloat(profile.VWAP)
			val := formatFloat(profile.VAL)
			vah := formatFloat(profile.VAH)
			entity.VWAP = &vwap
			entity.VAL = &val
			entity.VAH = &vah
		}

		if err := s.tickerHistory.Insert(ctx, entity); err != nil {
			return fmt.Errorf("insert ticker history for %s: %w", sessionDate.Format(time.DateOnly), err)
		}
	}

	return nil
}

func (s *Service) detectSession(ctx context.Context, info models.TickerInfo, sessionDate time.Time) (bool, *indicators.SessionProfile, error) {
	profile, err := s.calculator.CalculateMainSessionProfile(ctx, info.ID, sessionDate)
	if err != nil {
		if errors.Is(err, indicators.ErrNoTrades) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("calculate indicators for %s: %w", sessionDate.Format(time.DateOnly), err)
	}

	return true, &profile, nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
