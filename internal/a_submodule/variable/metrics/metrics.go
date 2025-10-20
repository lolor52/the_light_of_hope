package metrics

import (
	"errors"
	"math"
)

// RangeMetrics хранит значения VWAP и границ value area.
type RangeMetrics struct {
	VWAP float64
	VAL  float64
	VAH  float64
}

// CalculateRange рассчитывает VWAP и value area на основе цен и объёмов.
func CalculateRange(prices, volumes []float64) (RangeMetrics, error) {
	if len(prices) == 0 || len(volumes) == 0 {
		return RangeMetrics{}, errors.New("недостаточно данных для расчёта")
	}
	if len(prices) != len(volumes) {
		return RangeMetrics{}, errors.New("размеры массивов цен и объёмов не совпадают")
	}

	var totalVolume, weightedPrice float64
	for i := range prices {
		totalVolume += volumes[i]
		weightedPrice += prices[i] * volumes[i]
	}
	if totalVolume == 0 {
		return RangeMetrics{}, errors.New("суммарный объём равен нулю")
	}

	vwap := weightedPrice / totalVolume

	var variance float64
	for i := range prices {
		diff := prices[i] - vwap
		variance += volumes[i] * diff * diff
	}
	variance /= totalVolume
	sigma := math.Sqrt(variance)

	return RangeMetrics{
		VWAP: vwap,
		VAL:  vwap - sigma,
		VAH:  vwap + sigma,
	}, nil
}
