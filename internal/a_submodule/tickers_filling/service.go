package tickers_filling

import (
	"context"
	"errors"
	"fmt"
	"log"
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
		log.Printf("tickers_filling: параметры ограничивают выполнение: sessions_target=%d max_inactive_days=%d", s.config.SessionsTarget, s.config.MaxInactiveDays)
		return total, nil
	}

	log.Printf("tickers_filling: загрузка справочника тикеров")
	tickers, err := s.tickerInfoRepo.ListAll(ctx)
	if err != nil {
		return FillStats{}, fmt.Errorf("tickers filling: list tickers: %w", err)
	}
	log.Printf("tickers_filling: найдено тикеров: %d", len(tickers))

	startDate := s.yesterdayMoscow()
	log.Printf("tickers_filling: стартовая дата обработки: %s", startDate.Format("2006-01-02"))

	for _, ticker := range tickers {
		log.Printf("tickers_filling: начало обработки тикера %s", ticker.TickerName)
		stats, err := s.fillTicker(ctx, ticker, startDate)
		if err != nil {
			return FillStats{}, fmt.Errorf("tickers filling: %s: %w", ticker.TickerName, err)
		}
		total.add(stats)
		log.Printf(
			"tickers_filling: тикер %s обработан: существующих=%d созданных=%d активных_дат=%d неактивных_дат=%d",
			ticker.TickerName,
			stats.ExistingRecords,
			stats.CreatedRecords,
			stats.ActiveSessionDates,
			stats.InactiveSessionDates,
		)
	}

	return total, nil
}

func (s *Service) fillTicker(ctx context.Context, ticker models.TickerInfo, startDate time.Time) (FillStats, error) {
	var stats FillStats
	activeSessions := 0
	iterations := 0
	sessionDate := startDate

	log.Printf(
		"tickers_filling: %s: цель активных сессий=%d, максимум неактивных дней=%d",
		ticker.TickerName,
		s.config.SessionsTarget,
		s.config.MaxInactiveDays,
	)

	for iterations < s.config.MaxInactiveDays && activeSessions < s.config.SessionsTarget {
		if err := ctx.Err(); err != nil {
			return stats, err
		}

		dbDate := time.Date(sessionDate.Year(), sessionDate.Month(), sessionDate.Day(), 0, 0, 0, 0, time.UTC)
		log.Printf(
			"tickers_filling: %s: проверка даты %s (итерация=%d активных=%d)",
			ticker.TickerName,
			sessionDate.Format("2006-01-02"),
			iterations+1,
			activeSessions,
		)

		history, err := s.historyRepo.GetByDateAndName(ctx, ticker.TickerName, dbDate)
		if err == nil {
			stats.ExistingRecords++
			if history.TradingSessionActive {
				activeSessions++
				stats.ActiveSessionDates++
				log.Printf("tickers_filling: %s: запись уже существует и сессия активна", ticker.TickerName)
			} else {
				stats.InactiveSessionDates++
				log.Printf("tickers_filling: %s: запись уже существует и сессия неактивна", ticker.TickerName)
			}
		} else if errors.Is(err, db.ErrNotFound) {
			log.Printf("tickers_filling: %s: данных за дату %s нет, начинаем расчёт", ticker.TickerName, sessionDate.Format("2006-01-02"))
			sessionData, calcErr := s.computeSession(ctx, ticker.ID, sessionDate)
			if calcErr != nil {
				return stats, calcErr
			}

			if sessionData.active {
				activeSessions++
				stats.ActiveSessionDates++
				log.Printf("tickers_filling: %s: расчёт показал активную сессию", ticker.TickerName)
			} else {
				stats.InactiveSessionDates++
				log.Printf("tickers_filling: %s: расчёт показал неактивную сессию", ticker.TickerName)
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
			log.Printf("tickers_filling: %s: запись создана", ticker.TickerName)
		} else {
			return stats, fmt.Errorf("get ticker_history: %w", err)
		}

		iterations++
		sessionDate = sessionDate.AddDate(0, 0, -1)
		log.Printf(
			"tickers_filling: %s: переход к следующей дате %s",
			ticker.TickerName,
			sessionDate.Format("2006-01-02"),
		)

		if iterations < s.config.MaxInactiveDays && activeSessions < s.config.SessionsTarget {
			select {
			case <-time.After(500 * time.Millisecond):
				log.Printf("tickers_filling: %s: пауза между запросами завершена", ticker.TickerName)
			case <-ctx.Done():
				return stats, ctx.Err()
			}
		}
	}

	log.Printf(
		"tickers_filling: %s: цикл завершён после %d итераций (активных=%d)",
		ticker.TickerName,
		iterations,
		activeSessions,
	)

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

	log.Printf("tickers_filling: вычисление сессии для тикера %d за дату %s", tickerInfoID, sessionDate.Format("2006-01-02"))
	valueArea, err := s.valueAreaCalc.Calculate(ctx, tickerInfoID, sessionDate)
	if err != nil {
		if errors.Is(err, indicators.ErrNoTrades) {
			log.Printf("tickers_filling: для тикера %d нет сделок за дату %s", tickerInfoID, sessionDate.Format("2006-01-02"))
			return result, nil
		}
		return result, fmt.Errorf("calculate value area: %w", err)
	}

	result.active = true
	result.vwap = stringPointer(strconv.FormatFloat(valueArea.VWAP, 'f', 6, 64))
	result.val = stringPointer(strconv.FormatFloat(valueArea.VAL, 'f', 6, 64))
	result.vah = stringPointer(strconv.FormatFloat(valueArea.VAH, 'f', 6, 64))
	log.Printf(
		"tickers_filling: тикер %d: VWAP=%s VAL=%s VAH=%s",
		tickerInfoID,
		derefOrPlaceholder(result.vwap),
		derefOrPlaceholder(result.val),
		derefOrPlaceholder(result.vah),
	)

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

func derefOrPlaceholder(value *string) string {
	if value == nil {
		return "-"
	}
	return *value
}
