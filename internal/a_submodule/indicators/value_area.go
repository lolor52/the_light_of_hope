package indicators

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"invest_intraday/internal/a_submodule/moex"
	"invest_intraday/internal/a_technical/db"
)

// ErrNoTrades сигнализирует об отсутствии сделок основной сессии.
var ErrNoTrades = errors.New("no trades for main session")

// Calculator рассчитывает VAH/VAL/VWAP для основной торговой сессии.
type Calculator struct {
	tickerRepo *db.TickerInfoRepository
	issClient  *moex.ISSClient

	boardCache map[string]moex.BoardMetadata
	cacheMu    sync.Mutex
}

// Result содержит рассчитанные метрики торговой сессии.
type Result struct {
	VWAP float64
	VAL  float64
	VAH  float64
}

var (
	moscowOnce sync.Once
	moscowLoc  *time.Location
)

// NewCalculator создаёт сервис расчёта метрик.
func NewCalculator(tickerRepo *db.TickerInfoRepository, issClient *moex.ISSClient) *Calculator {
	return &Calculator{
		tickerRepo: tickerRepo,
		issClient:  issClient,
		boardCache: make(map[string]moex.BoardMetadata),
	}
}

// Calculate рассчитывает VAH/VAL/VWAP по идентификатору тикера и дате.
func (c *Calculator) Calculate(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (Result, error) {
	if c.tickerRepo == nil || c.issClient == nil {
		return Result{}, fmt.Errorf("calculator is not fully configured")
	}

	info, err := c.tickerRepo.GetByID(ctx, tickerInfoID)
	if err != nil {
		return Result{}, fmt.Errorf("load ticker info: %w", err)
	}

	board, err := c.boardMetadata(ctx, info.BoardID)
	if err != nil {
		return Result{}, fmt.Errorf("load board metadata: %w", err)
	}

	trades, err := c.issClient.Trades(ctx, board, info.BoardID, info.SecID, sessionDate)
	if err != nil {
		return Result{}, fmt.Errorf("load trades: %w", err)
	}

	mainTrades, err := filterMainSession(trades)
	if err != nil {
		return Result{}, err
	}

	vwap, err := calculateVWAP(mainTrades)
	if err != nil {
		return Result{}, err
	}

	val, vah := calculateValueArea(mainTrades)

	return Result{
		VWAP: vwap,
		VAL:  val,
		VAH:  vah,
	}, nil
}

func (c *Calculator) boardMetadata(ctx context.Context, boardID string) (moex.BoardMetadata, error) {
	boardID = strings.ToUpper(boardID)

	c.cacheMu.Lock()
	board, ok := c.boardCache[boardID]
	c.cacheMu.Unlock()
	if ok {
		return board, nil
	}

	board, err := c.issClient.BoardMetadata(ctx, boardID)
	if err != nil {
		return moex.BoardMetadata{}, err
	}

	c.cacheMu.Lock()
	c.boardCache[boardID] = board
	c.cacheMu.Unlock()

	return board, nil
}

func filterMainSession(trades []moex.Trade) ([]moex.Trade, error) {
	if len(trades) == 0 {
		return nil, ErrNoTrades
	}

	var filtered []moex.Trade
	for _, trade := range trades {
		if isMainSessionTrade(trade) {
			filtered = append(filtered, trade)
		}
	}

	if len(filtered) == 0 {
		return nil, ErrNoTrades
	}

	return filtered, nil
}

func isMainSessionTrade(trade moex.Trade) bool {
	session := strings.ToUpper(strings.TrimSpace(trade.TradingSession))
	switch session {
	case "P6", "EVENING", "NIGHT", "AFTERHOURS":
		return false
	case "P0", "MORNING":
		return false
	case "DAY", "P1", "2":
		return true
	}

	tradeTime, err := parseMoscowTime(trade.TradeTime)
	if err != nil {
		return false
	}

	start := time.Date(0, 1, 1, 10, 0, 0, 0, tradeTime.Location())
	end := time.Date(0, 1, 1, 18, 50, 0, 0, tradeTime.Location())
	return !tradeTime.Before(start) && tradeTime.Before(end)
}

func parseMoscowTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}

	moscowOnce.Do(func() {
		loc, err := time.LoadLocation("Europe/Moscow")
		if err != nil {
			moscowLoc = time.FixedZone("MSK", 3*60*60)
			return
		}
		moscowLoc = loc
	})
	if moscowLoc == nil {
		return time.Time{}, fmt.Errorf("moscow location unavailable")
	}

	parsed, err := time.ParseInLocation("15:04:05", value, moscowLoc)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func calculateVWAP(trades []moex.Trade) (float64, error) {
	var totalValue float64
	var totalVolume float64

	for _, trade := range trades {
		if trade.Quantity <= 0 {
			continue
		}
		value := trade.Value
		if value == 0 {
			value = trade.Price * trade.Quantity
		}
		totalValue += value
		totalVolume += trade.Quantity
	}

	if totalVolume == 0 {
		return 0, ErrNoTrades
	}

	return totalValue / totalVolume, nil
}

func calculateValueArea(trades []moex.Trade) (float64, float64) {
	type level struct {
		Price  float64
		Volume float64
	}

	volumeByPrice := make(map[float64]float64)
	var totalVolume float64
	for _, trade := range trades {
		if trade.Quantity <= 0 {
			continue
		}
		volumeByPrice[trade.Price] += trade.Quantity
		totalVolume += trade.Quantity
	}

	if totalVolume == 0 {
		return 0, 0
	}

	levels := make([]level, 0, len(volumeByPrice))
	for price, volume := range volumeByPrice {
		levels = append(levels, level{Price: price, Volume: volume})
	}
	sort.Slice(levels, func(i, j int) bool {
		if levels[i].Price == levels[j].Price {
			return levels[i].Volume > levels[j].Volume
		}
		return levels[i].Price < levels[j].Price
	})

	if len(levels) == 0 {
		return 0, 0
	}

	pocIndex := 0
	maxVolume := levels[0].Volume
	for i := 1; i < len(levels); i++ {
		if levels[i].Volume > maxVolume {
			maxVolume = levels[i].Volume
			pocIndex = i
		}
	}

	minIndex, maxIndex := pocIndex, pocIndex
	accumulated := levels[pocIndex].Volume
	target := totalVolume * 0.7
	lower := pocIndex - 1
	upper := pocIndex + 1

	for accumulated < target && (lower >= 0 || upper < len(levels)) {
		lowerVolume := volumeAt(levels, lower)
		upperVolume := volumeAt(levels, upper)

		if lowerVolume == 0 && upperVolume == 0 {
			if lower >= 0 {
				minIndex = lower
				lower--
			}
			if upper < len(levels) {
				maxIndex = upper
				upper++
			}
			continue
		}

		if upperVolume > lowerVolume {
			accumulated += upperVolume
			maxIndex = upper
			upper++
		} else {
			accumulated += lowerVolume
			minIndex = lower
			lower--
		}
	}

	val := levels[minIndex].Price
	vah := levels[maxIndex].Price

	if val > vah {
		val, vah = vah, val
	}

	return val, vah
}

func volumeAt(levels []level, index int) float64 {
	if index < 0 || index >= len(levels) {
		return 0
	}
	return levels[index].Volume
}

// RoundResult округляет значения до указанного количества знаков.
func RoundResult(value float64, digits int) float64 {
	if digits < 0 {
		return value
	}
	factor := math.Pow(10, float64(digits))
	return math.Round(value*factor) / factor
}
