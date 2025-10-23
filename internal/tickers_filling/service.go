package tickers_filling

import (
	"context"
	"errors"
	"fmt"
	"log"
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

// FillStats содержит статистику по обработанным торговым сессиям.
type FillStats struct {
	ExistingEntries  int
	CreatedEntries   int
	ActiveSessions   int
	InactiveSessions int
}

// Fill загружает недостающие данные об основных торговых сессиях для тикеров TQBR.
func (s *Service) Fill(ctx context.Context) (FillStats, error) {
	var summary FillStats

	if s == nil {
		return summary, errors.New("tickers_filling: nil service")
	}

	log.Printf("tickers_filling: start filling job")

	tickers, err := s.tickerInfos.ListAll(ctx)
	if err != nil {
		log.Printf("tickers_filling: list ticker info error: %v", err)
		return summary, fmt.Errorf("tickers_filling: list ticker info: %w", err)
	}

	log.Printf("tickers_filling: received %d tickers", len(tickers))
	startDate := s.yesterdayInMoscow()
	log.Printf("tickers_filling: initial session date %s", startDate.Format("2006-01-02"))

	for _, info := range tickers {
		log.Printf("tickers_filling: processing ticker %s (%s)", info.TickerName, info.BoardID)
		if info.BoardID != "TQBR" {
			log.Printf("tickers_filling: skip ticker %s due to board %s", info.TickerName, info.BoardID)
			continue
		}

		stats, err := s.fillTickerHistory(ctx, info, startDate)
		if err != nil {
			log.Printf("tickers_filling: error processing %s: %v", info.TickerName, err)
			return summary, err
		}

		summary.ExistingEntries += stats.ExistingEntries
		summary.CreatedEntries += stats.CreatedEntries
		summary.ActiveSessions += stats.ActiveSessions
		summary.InactiveSessions += stats.InactiveSessions

		log.Printf("tickers_filling: ticker %s stats existing=%d created=%d active=%d inactive=%d",
			info.TickerName, stats.ExistingEntries, stats.CreatedEntries, stats.ActiveSessions, stats.InactiveSessions)
	}

	log.Printf("tickers_filling: summary existing=%d created=%d active=%d inactive=%d", summary.ExistingEntries, summary.CreatedEntries, summary.ActiveSessions, summary.InactiveSessions)

	return summary, nil
}

func (s *Service) yesterdayInMoscow() time.Time {
	now := s.now()
	localNow := now.In(s.moscowLocation)
	midnight := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, s.moscowLocation)

	return midnight.AddDate(0, 0, -1)
}

func (s *Service) fillTickerHistory(ctx context.Context, info models.TickerInfo, startDate time.Time) (FillStats, error) {
	var stats FillStats

	if s.sessionsLimit <= 0 {
		log.Printf("tickers_filling: sessions limit is zero for %s", info.TickerName)
		return stats, nil
	}
	if s.maxInactiveDays <= 0 {
		log.Printf("tickers_filling: max inactive days is zero for %s", info.TickerName)
		return stats, nil
	}

	activeSessions := 0

	for iteration := 0; iteration < s.maxInactiveDays && activeSessions < s.sessionsLimit; iteration++ {
		sessionDate := startDate.AddDate(0, 0, -iteration)

		log.Printf("tickers_filling: check history %s %s", info.TickerName, sessionDate.Format("2006-01-02"))

		history, err := s.tickerHistory.GetByDateAndName(ctx, info.TickerName, sessionDate)
		if err == nil {
			log.Printf("tickers_filling: found history %s %s active=%t", info.TickerName, sessionDate.Format("2006-01-02"), history.TradingSessionActive)
			stats.ExistingEntries++
			if history.TradingSessionActive {
				activeSessions++
				stats.ActiveSessions++
			} else {
				stats.InactiveSessions++
			}
			continue
		}
		if !errors.Is(err, db.ErrNotFound) {
			log.Printf("tickers_filling: load history error %s %s: %v", info.TickerName, sessionDate.Format("2006-01-02"), err)
			return stats, fmt.Errorf("tickers_filling: load history %s %s: %w", info.TickerName, sessionDate.Format("2006-01-02"), err)
		}

		log.Printf("tickers_filling: history missing %s %s, requesting indicators", info.TickerName, sessionDate.Format("2006-01-02"))
		profile, err := s.calculator.CalculateMainSessionProfile(ctx, info.ID, sessionDate)
		response := alorResponseText(s.calculator)
		if response != "" {
			log.Printf("tickers_filling: Alor response %s %s: %s", info.TickerName, sessionDate.Format("2006-01-02"), response)
		}
		sessionActive := false
		var vwap, val, vah *string

		switch {
		case err == nil:
			log.Printf("tickers_filling: indicators ready %s %s vwap=%f val=%f vah=%f", info.TickerName, sessionDate.Format("2006-01-02"), profile.VWAP, profile.VAL, profile.VAH)
			sessionActive = true
			vwap = floatToPtr(profile.VWAP)
			val = floatToPtr(profile.VAL)
			vah = floatToPtr(profile.VAH)
			activeSessions++
			stats.ActiveSessions++
		case errors.Is(err, indicators.ErrNoTrades):
			log.Printf("tickers_filling: no trades for %s %s", info.TickerName, sessionDate.Format("2006-01-02"))
			sessionActive = false
			stats.InactiveSessions++
		default:
			log.Printf("tickers_filling: indicator calculation error %s %s: %v", info.TickerName, sessionDate.Format("2006-01-02"), err)
			return stats, fmt.Errorf("tickers_filling: calculate session %s %s: %w", info.TickerName, sessionDate.Format("2006-01-02"), err)
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

		log.Printf("tickers_filling: inserting history %s %s active=%t", info.TickerName, sessionDate.Format("2006-01-02"), sessionActive)
		if err := s.tickerHistory.Insert(ctx, entity); err != nil {
			log.Printf("tickers_filling: insert error %s %s: %v", info.TickerName, sessionDate.Format("2006-01-02"), err)
			return stats, fmt.Errorf("tickers_filling: insert history %s %s: %w", info.TickerName, sessionDate.Format("2006-01-02"), err)
		}
		stats.CreatedEntries++
		log.Printf("tickers_filling: history inserted %s %s", info.TickerName, sessionDate.Format("2006-01-02"))
	}

	log.Printf("tickers_filling: finished ticker %s iterations=%d activeSessions=%d", info.TickerName, stats.CreatedEntries+stats.ExistingEntries, stats.ActiveSessions)

	return stats, nil
}

func alorResponseText(calculator sessionCalculator) string {
	type responder interface {
		LastAlorResponse() string
	}
	if calcResponder, ok := calculator.(responder); ok {
		return calcResponder.LastAlorResponse()
	}

	return ""
}

func floatToPtr(value float64) *string {
	text := strconv.FormatFloat(value, 'f', -1, 64)

	return &text
}
