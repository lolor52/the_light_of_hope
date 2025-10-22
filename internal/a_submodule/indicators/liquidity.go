package indicators

import (
	"fmt"
	"math"

	"invest_intraday/internal/a_submodule/moex"
)

// LiquidityMetrics содержит промежуточные расчёты ликвидности.
type LiquidityMetrics struct {
	TotalValue      float64
	TotalVolume     float64
	VWAP            float64
	High            float64
	Low             float64
	RelativeRange   float64
	SimpleLiquidity float64
	LogLiquidity    float64
}

// CalculateLiquidityMetrics вычисляет базовую метрику ликвидности на основе сделок основной сессии.
func CalculateLiquidityMetrics(trades []moex.Trade) (LiquidityMetrics, error) {
	metrics := LiquidityMetrics{}
	filtered := make([]moex.Trade, 0, len(trades))
	for _, trade := range trades {
		if trade.Price <= 0 || trade.Quantity <= 0 {
			continue
		}
		filtered = append(filtered, trade)
	}
	if len(filtered) == 0 {
		return metrics, fmt.Errorf("liquidity: no trades in main session")
	}

	highPrice := filtered[0].Price
	lowPrice := filtered[0].Price
	var totalValue float64
	var totalVolume float64
	for _, trade := range filtered {
		if trade.Price > highPrice {
			highPrice = trade.Price
		}
		if trade.Price < lowPrice {
			lowPrice = trade.Price
		}
		totalValue += trade.Price * trade.Quantity
		totalVolume += trade.Quantity
	}
	if totalVolume <= 0 {
		return metrics, fmt.Errorf("liquidity: zero volume")
	}

	vwap := totalValue / totalVolume
	if vwap <= 0 {
		return metrics, fmt.Errorf("liquidity: invalid VWAP")
	}

	relRange := (highPrice - lowPrice) / vwap
	if relRange < 0 {
		relRange = 0
	}

	simple := totalValue / (relRange + machineEpsilon)
	logLiquidity := math.Log1p(simple)

	metrics.TotalValue = totalValue
	metrics.TotalVolume = totalVolume
	metrics.VWAP = vwap
	metrics.High = highPrice
	metrics.Low = lowPrice
	metrics.RelativeRange = relRange
	metrics.SimpleLiquidity = simple
	metrics.LogLiquidity = logLiquidity
	return metrics, nil
}

// CrossSectionScore переводит значения лог-линейной ликвидности в баллы 0-100 по формуле 3.A.
func CrossSectionScore(values []float64) []float64 {
	cleaned := filterFinite(values)
	if len(cleaned) == 0 {
		return make([]float64, len(values))
	}
	a := quantile(cleaned, 0.10)
	b := quantile(cleaned, 0.90)
	result := make([]float64, len(values))
	denom := b - a
	for i, value := range values {
		if math.IsNaN(value) {
			result[i] = math.NaN()
			continue
		}
		if denom == 0 {
			result[i] = clip(0.5*100, 0, 100)
			continue
		}
		normalized := (value - a) / denom
		result[i] = 100 * clip(normalized, 0, 1)
	}
	return result
}
