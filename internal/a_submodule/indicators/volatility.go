package indicators

import (
	"fmt"
	"math"
	"sort"

	"invest_intraday/internal/a_submodule/moex"
)

var (
	parkinsonDenominator = 2 * math.Sqrt(math.Log(2))
)

// VolatilityMetrics содержит промежуточные значения волатильности.
type VolatilityMetrics struct {
	Parkinson      float64
	RogersSatchell float64
	Score          float64
}

// CalculateVolatilityMetrics рассчитывает дневную волатильность по формулам Parkinson и Rogers–Satchell.
// На вход ожидаются сделки основной сессии, отсортированные по времени.
func CalculateVolatilityMetrics(trades []moex.Trade) (VolatilityMetrics, error) {
	metrics := VolatilityMetrics{}
	filtered := make([]moex.Trade, 0, len(trades))
	for _, trade := range trades {
		if trade.Price <= 0 {
			continue
		}
		filtered = append(filtered, trade)
	}
	if len(filtered) == 0 {
		return metrics, fmt.Errorf("volatility: no trades in main session")
	}

	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Time.Before(filtered[j].Time) })

	openPrice := filtered[0].Price
	closePrice := filtered[len(filtered)-1].Price
	highPrice := openPrice
	lowPrice := openPrice
	for _, trade := range filtered {
		if trade.Price > highPrice {
			highPrice = trade.Price
		}
		if trade.Price < lowPrice {
			lowPrice = trade.Price
		}
	}

	if highPrice < math.Max(openPrice, closePrice) || lowPrice > math.Min(openPrice, closePrice) {
		return metrics, fmt.Errorf("volatility: inconsistent high/low data")
	}

	park := 0.0
	if highPrice > 0 && lowPrice > 0 {
		ratio := highPrice / lowPrice
		park = math.Abs(math.Log(ratio))
		park /= parkinsonDenominator
	}
	metrics.Parkinson = park

	rsComponent := math.Log(highPrice/closePrice)*math.Log(highPrice/openPrice) +
		math.Log(lowPrice/closePrice)*math.Log(lowPrice/openPrice)
	if rsComponent < 0 {
		rsComponent = 0
	}
	metrics.RogersSatchell = math.Sqrt(rsComponent)

	sigma := metrics.RogersSatchell
	if sigma == 0 {
		sigma = metrics.Parkinson
	}
	metrics.Score = clip(sigma*100, 0, 100)
	return metrics, nil
}
