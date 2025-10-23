package indicators

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"invest_intraday/internal/a_submodule/moex"
	"invest_intraday/models"
)

// ErrNoTradesFound возвращается, когда нет сделок основной сессии.
var ErrNoTradesFound = errors.New("нет сделок в основной торговой сессии")

// TickerInfoProvider описывает источник данных о тикерах.
type TickerInfoProvider interface {
	GetByID(ctx context.Context, id int64) (models.TickerInfo, error)
}

// TradesProvider описывает источник сделок MOEX.
type TradesProvider interface {
	Trades(ctx context.Context, boardID, secID string, date time.Time) ([]moex.Trade, error)
}

// ValueAreaMetrics содержит рассчитанные показатели основной сессии.
type ValueAreaMetrics struct {
	VAH  float64
	VAL  float64
	VWAP float64
}

// ValueAreaCalculator рассчитывает VAH/VAL/VWAP по данным MOEX ISS.
type ValueAreaCalculator struct {
	tickerProvider TickerInfoProvider
	tradesProvider TradesProvider
}

// NewValueAreaCalculator создаёт калькулятор значений value area.
func NewValueAreaCalculator(tickerProvider TickerInfoProvider, tradesProvider TradesProvider) (*ValueAreaCalculator, error) {
	if tickerProvider == nil {
		return nil, errors.New("ticker provider is nil")
	}
	if tradesProvider == nil {
		return nil, errors.New("trades provider is nil")
	}

	return &ValueAreaCalculator{
		tickerProvider: tickerProvider,
		tradesProvider: tradesProvider,
	}, nil
}

// Calculate вычисляет показатели основной сессии за указанную дату.
func (c *ValueAreaCalculator) Calculate(ctx context.Context, tickerInfoID int64, date time.Time) (ValueAreaMetrics, error) {
	if c == nil {
		return ValueAreaMetrics{}, errors.New("calculator is nil")
	}

	ticker, err := c.tickerProvider.GetByID(ctx, tickerInfoID)
	if err != nil {
		return ValueAreaMetrics{}, fmt.Errorf("load ticker info: %w", err)
	}

	trades, err := c.tradesProvider.Trades(ctx, ticker.BoardID, ticker.SecID, date)
	if err != nil {
		return ValueAreaMetrics{}, fmt.Errorf("load trades: %w", err)
	}

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*60*60)
	}

	sessionStart, sessionEnd := mainSessionBounds(date.In(loc))

	volumeByPrice := make(map[float64]float64)
	var totalVolume float64
	var totalPriceVolume float64

	for _, trade := range trades {
		if trade.Time.Before(sessionStart) || trade.Time.After(sessionEnd) {
			continue
		}
		if trade.Quantity <= 0 {
			continue
		}

		volumeByPrice[trade.Price] += trade.Quantity
		totalVolume += trade.Quantity
		totalPriceVolume += trade.Price * trade.Quantity
	}

	if totalVolume == 0 {
		return ValueAreaMetrics{}, ErrNoTradesFound
	}

	prices := make([]float64, 0, len(volumeByPrice))
	for price := range volumeByPrice {
		prices = append(prices, price)
	}
	sort.Float64s(prices)

	pocIndex := pointOfControlIndex(prices, volumeByPrice)
	if pocIndex == -1 {
		return ValueAreaMetrics{}, ErrNoTradesFound
	}

	valIdx, vahIdx := expandValueArea(prices, volumeByPrice, pocIndex, totalVolume)

	metrics := ValueAreaMetrics{
		VAL:  prices[valIdx],
		VAH:  prices[vahIdx],
		VWAP: totalPriceVolume / totalVolume,
	}

	return metrics, nil
}

func mainSessionBounds(day time.Time) (time.Time, time.Time) {
	loc := day.Location()
	start := time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), 18, 45, 0, 0, loc)
	return start, end
}

func pointOfControlIndex(prices []float64, volumes map[float64]float64) int {
	if len(prices) == 0 {
		return -1
	}

	pocIndex := 0
	maxVolume := volumes[prices[0]]

	for i := 1; i < len(prices); i++ {
		vol := volumes[prices[i]]
		if vol > maxVolume {
			maxVolume = vol
			pocIndex = i
		}
	}

	return pocIndex
}

func expandValueArea(prices []float64, volumes map[float64]float64, pocIndex int, totalVolume float64) (int, int) {
	targetVolume := totalVolume * 0.7
	currentVolume := volumes[prices[pocIndex]]

	lowIdx := pocIndex
	highIdx := pocIndex

	for currentVolume < targetVolume {
		nextLowIdx := lowIdx - 1
		nextHighIdx := highIdx + 1

		var nextLowVolume float64
		if nextLowIdx >= 0 {
			nextLowVolume = volumes[prices[nextLowIdx]]
		}

		var nextHighVolume float64
		if nextHighIdx < len(prices) {
			nextHighVolume = volumes[prices[nextHighIdx]]
		}

		if nextLowVolume == 0 && nextHighVolume == 0 {
			break
		}

		if nextLowVolume >= nextHighVolume && nextLowIdx >= 0 {
			lowIdx = nextLowIdx
			currentVolume += nextLowVolume
			continue
		}

		if nextHighIdx < len(prices) {
			highIdx = nextHighIdx
			currentVolume += nextHighVolume
			continue
		}

		break
	}

	return lowIdx, highIdx
}
