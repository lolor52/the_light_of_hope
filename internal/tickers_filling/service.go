package tickers_filling

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"invest_intraday/internal/a_technical/db"
	"invest_intraday/internal/indicators"
	"invest_intraday/models"
)

// tickerInfoLister описывает зависимость для получения перечня тикеров.
type tickerInfoLister interface {
	ListAll(ctx context.Context) ([]models.TickerInfo, error)
}

// tickerHistoryStore инкапсулирует операции чтения и записи истории тикера.
type tickerHistoryStore interface {
	GetByDateAndName(ctx context.Context, name string, sessionDate time.Time) (models.TickerHistory, error)
	Insert(ctx context.Context, entity models.TickerHistory) error
}

// sessionCalculator рассчитывает показатели основной торговой сессии.
type sessionCalculator interface {
	CalculateMainSessionProfile(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (indicators.SessionProfile, error)
}

// Service отвечает за заполнение данных об исторических торговых сессиях тикеров.
type Service struct {
	tickerInfos     tickerInfoLister
	tickerHistory   tickerHistoryStore
	calculator      sessionCalculator
	sessionsLimit   int
	maxInactiveDays int
	now             func() time.Time
	moscowLocation  *time.Location
}

// NewService создаёт сервис заполнения истории тикеров.
func NewService(infoRepo tickerInfoLister, historyRepo tickerHistoryStore, calc sessionCalculator, sessionsLimit, maxInactiveDays int) (*Service, error) {
	if infoRepo == nil {
		return nil, errors.New("tickers_filling: ticker info repository is required")
	}
	if historyRepo == nil {
		return nil, errors.New("tickers_filling: ticker history repository is required")
	}
	if calc == nil {
		return nil, errors.New("tickers_filling: session calculator is required")
	}
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return nil, fmt.Errorf("tickers_filling: load moscow location: %w", err)
	}

	svc := &Service{
		tickerInfos:     infoRepo,
		tickerHistory:   historyRepo,
		calculator:      calc,
		sessionsLimit:   sessionsLimit,
		maxInactiveDays: maxInactiveDays,
		now:             time.Now,
		moscowLocation:  loc,
	}

	return svc, nil
}

// Fill загружает недостающие данные об основных торговых сессиях для тикеров TQBR.
func (s *Service) Fill(ctx context.Context) error {
	if s == nil {
		return errors.New("tickers_filling: nil service")
	}

	tickers, err := s.tickerInfos.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("tickers_filling: list ticker info: %w", err)
	}

	startDate := s.yesterdayInMoscow()

	for _, info := range tickers {
		if info.BoardID != "TQBR" {
			continue
		}

		if err := s.fillTickerHistory(ctx, info, startDate); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) yesterdayInMoscow() time.Time {
	now := s.now()
	localNow := now.In(s.moscowLocation)
	midnight := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, s.moscowLocation)

	return midnight.AddDate(0, 0, -1)
}

func (s *Service) fillTickerHistory(ctx context.Context, info models.TickerInfo, startDate time.Time) error {
	if s.sessionsLimit <= 0 {
		return nil
	}
	if s.maxInactiveDays <= 0 {
		return nil
	}

	activeSessions := 0

	for iteration := 0; iteration < s.maxInactiveDays && activeSessions < s.sessionsLimit; iteration++ {
		sessionDate := startDate.AddDate(0, 0, -iteration)

		history, err := s.tickerHistory.GetByDateAndName(ctx, info.TickerName, sessionDate)
		if err == nil {
			if history.TradingSessionActive {
				activeSessions++
			}
			continue
		}
		if !errors.Is(err, db.ErrNotFound) {
			return fmt.Errorf("tickers_filling: load history %s %s: %w", info.TickerName, sessionDate.Format("2006-01-02"), err)
		}

		profile, err := s.calculator.CalculateMainSessionProfile(ctx, info.ID, sessionDate)
		sessionActive := false
		var vwap, val, vah *string

		switch {
		case err == nil:
			sessionActive = true
			vwap = floatToPtr(profile.VWAP)
			val = floatToPtr(profile.VAL)
			vah = floatToPtr(profile.VAH)
			activeSessions++
		case errors.Is(err, indicators.ErrNoTrades):
			sessionActive = false
		default:
			return fmt.Errorf("tickers_filling: calculate session %s %s: %w", info.TickerName, sessionDate.Format("2006-01-02"), err)
		}

		entity := models.TickerHistory{
			TradingSessionDate:   sessionDate,
			TradingSessionActive: sessionActive,
			TickerInfoID:         info.ID,
			VWAP:                 vwap,
			VAL:                  val,
			VAH:                  vah,
			SwingCountPaired:     nil,
		}

		if err := s.tickerHistory.Insert(ctx, entity); err != nil {
			return fmt.Errorf("tickers_filling: insert history %s %s: %w", info.TickerName, sessionDate.Format("2006-01-02"), err)
		}
	}

	return nil
}

func floatToPtr(value float64) *string {
	text := strconv.FormatFloat(value, 'f', -1, 64)

	return &text
}
