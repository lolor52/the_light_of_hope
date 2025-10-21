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

// RunStats содержит статистику выполнения сервиса.
type RunStats struct {
	Existing int
	Created  int
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

	for _, tickerCfg := range s.cfg.MOEXTickers {
		tickerStats, tickerPending, err := s.processTicker(ctx, tickerCfg, startDate)
		stats.Existing += tickerStats.Existing
		stats.Created += tickerStats.Created
		if err != nil {
			log.Printf("tickers_filling: ошибка при обработке тикера %s: %v", tickerCfg.Ticker, err)
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

func (s *Service) processTicker(ctx context.Context, tickerCfg config.MOEXTicker, startDate time.Time) (RunStats, []pendingRecord, error) {
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

		historyRow, err := s.moexClient.GetHistoryRow(ctx, tickerCfg.BoardID, tickerCfg.SecID, date)
		if err != nil {
			return stats, nil, fmt.Errorf("get history row: %w", err)
		}

		sessionActive := historyRow != nil && historyRow.Volume > 0

		if sessionActive {
			consecutiveInactiveDays = 0
		} else if maxInactiveDays > 0 {
			consecutiveInactiveDays++
		}

		_, err = s.repo.GetByDateAndName(ctx, tickerCfg.Ticker, date)
		if err == nil {
			stats.Existing++
			if sessionActive {
				tradingSessionsFound++
			}
			if !sessionActive && maxInactiveDays > 0 && consecutiveInactiveDays >= maxInactiveDays {
				log.Printf("tickers_filling: тикер %s не торговался последние %d дней, остановлено заполнение", tickerCfg.Ticker, maxInactiveDays)
				return stats, pending, nil
			}
			// запись уже есть, перейдём к следующей дате
			date = date.AddDate(0, 0, -1)
			continue
		}
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return stats, nil, err
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
				if insertErr := s.repo.Insert(ctx, entity); insertErr != nil {
					return stats, nil, insertErr
				}
				stats.Created++
			}
			if maxInactiveDays > 0 && consecutiveInactiveDays >= maxInactiveDays {
				log.Printf("tickers_filling: тикер %s не торговался последние %d дней, остановлено заполнение", tickerCfg.Ticker, maxInactiveDays)
				return stats, pending, nil
			}
			date = date.AddDate(0, 0, -1)
			continue
		}

		tradingSessionsFound++

		metrics, err := s.collectMetrics(ctx, tickerCfg, securityInfo, *historyRow)
		if err != nil {
			log.Printf("tickers_filling: не удалось рассчитать метрики для %s %s: %v", tickerCfg.Ticker, date.Format("2006-01-02"), err)
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
	FlatTrend  *flatTrendComponents
	Volatility *volatilityComponents
	Liquidity  *liquidityComponents
}

type pendingRecord struct {
	entity models.Ticker
	raw    sessionRawMetrics
}

func (s *Service) collectMetrics(ctx context.Context, tickerCfg config.MOEXTicker, securityInfo moex.SecurityInfo, historyRow moex.HistoryRow) (sessionRawMetrics, error) {
	metrics := sessionRawMetrics{}

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
		filterValue, err := calculateFlatTrendComponents(candles, historyRow, *prevRow)
		if err == nil {
			metrics.FlatTrend = &filterValue
		}
	}

	historyWindow, err := s.moexClient.GetHistoryWindow(ctx, tickerCfg.BoardID, tickerCfg.SecID, historyRow.TradeDate.AddDate(0, 0, -60), historyRow.TradeDate)
	if err == nil && len(historyWindow) > 0 {
		volValue, err := calculateVolatilityComponents(historyRow, candles, historyWindow)
		if err == nil {
			metrics.Volatility = &volValue
		}
	}

	marketData, err := s.moexClient.GetMarketData(ctx, tickerCfg.BoardID, tickerCfg.SecID, historyRow.TradeDate)
	if err == nil {
		orderBook, err := s.moexClient.GetOrderBook(ctx, tickerCfg.BoardID, tickerCfg.SecID, historyRow.TradeDate, 5)
		if err == nil {
			liquidityValue, err := calculateLiquidityComponents(candles, marketData, orderBook, securityInfo)
			if err == nil {
				metrics.Liquidity = &liquidityValue
			}
		}
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
		applyFlatTrendNormalization(records)
		applyVolatilityNormalization(records)
		applyLiquidityNormalization(records)
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

func applyFlatTrendNormalization(records []*pendingRecord) {
	type item struct {
		rec *pendingRecord
		cmp *flatTrendComponents
	}

	var items []item
	for _, record := range records {
		if record.raw.FlatTrend == nil {
			continue
		}
		items = append(items, item{rec: record, cmp: record.raw.FlatTrend})
	}
	if len(items) == 0 {
		return
	}

	overlaps := make([]float64, len(items))
	slopes := make([]float64, len(items))
	skews := make([]float64, len(items))
	for i, it := range items {
		overlaps[i] = it.cmp.OverlapPercent
		slopes[i] = it.cmp.SlopeAbs
		skews[i] = it.cmp.SkewPercent
	}

	overlapMin, overlapMax := minMax(overlaps)
	slopeMin, slopeMax := minMax(slopes)
	skewMin, skewMax := minMax(skews)

	for i, it := range items {
		overlapNorm := normUp(overlaps[i], overlapMin, overlapMax)
		slopeNorm := normDown(slopes[i], slopeMin, slopeMax)
		skewNorm := normDown(skews[i], skewMin, skewMax)
		value := 100 * (0.5*overlapNorm + 0.3*slopeNorm + 0.2*skewNorm)
		it.rec.entity.FlatTrendFilter = formatFloat(value)
	}
}

func applyVolatilityNormalization(records []*pendingRecord) {
	type item struct {
		rec *pendingRecord
		cmp *volatilityComponents
	}

	var items []item
	for _, record := range records {
		if record.raw.Volatility == nil {
			continue
		}
		items = append(items, item{rec: record, cmp: record.raw.Volatility})
	}
	if len(items) == 0 {
		return
	}

	atrValues := make([]float64, len(items))
	rvolValues := make([]float64, len(items))
	for i, it := range items {
		atrValues[i] = it.cmp.ATRPercent
		rvolValues[i] = it.cmp.RelativeVolume
	}

	atrMin, atrMax := minMax(atrValues)
	rvolMin, rvolMax := minMax(rvolValues)

	for i, it := range items {
		atrNorm := normUp(atrValues[i], atrMin, atrMax)
		rvolNorm := normUp(rvolValues[i], rvolMin, rvolMax)
		value := 100 * (0.6*atrNorm + 0.4*rvolNorm)
		it.rec.entity.Volatility = formatFloat(value)
	}
}

func applyLiquidityNormalization(records []*pendingRecord) {
	type item struct {
		rec *pendingRecord
		cmp *liquidityComponents
	}

	var items []item
	for _, record := range records {
		if record.raw.Liquidity == nil {
			continue
		}
		items = append(items, item{rec: record, cmp: record.raw.Liquidity})
	}
	if len(items) == 0 {
		return
	}

	spreadValues := make([]float64, len(items))
	turnoverValues := make([]float64, len(items))
	depthValues := make([]float64, len(items))
	tickValues := make([]float64, len(items))
	for i, it := range items {
		spreadValues[i] = it.cmp.SpreadRelativePercent
		turnoverValues[i] = it.cmp.TurnoverPerMinute
		depthValues[i] = it.cmp.DepthTopFive
		tickValues[i] = it.cmp.TickPercent
	}

	spreadMin, spreadMax := minMax(spreadValues)
	turnoverMin, turnoverMax := minMax(turnoverValues)
	depthMin, depthMax := minMax(depthValues)
	tickMin, tickMax := minMax(tickValues)

	for i, it := range items {
		spreadComponent := normDown(spreadValues[i], spreadMin, spreadMax)
		turnoverComponent := normUp(turnoverValues[i], turnoverMin, turnoverMax)
		depthComponent := normUp(depthValues[i], depthMin, depthMax)
		tickComponent := normDown(tickValues[i], tickMin, tickMax)
		value := 100 * (0.35*spreadComponent + 0.35*turnoverComponent + 0.25*depthComponent + 0.05*tickComponent)
		it.rec.entity.Liquidity = formatFloat(value)
	}
}

func minMax(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	minVal := values[0]
	maxVal := values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	return minVal, maxVal
}

func normUp(value, minVal, maxVal float64) float64 {
	return (value - minVal) / (maxVal - minVal + epsilon)
}

func normDown(value, minVal, maxVal float64) float64 {
	return 1 - normUp(value, minVal, maxVal)
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
