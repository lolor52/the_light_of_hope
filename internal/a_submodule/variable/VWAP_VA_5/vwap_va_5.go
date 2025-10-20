package vwapva5

import (
	"errors"

	"invest_intraday/internal/a_submodule/variable/metrics"
)

// Calculate рассчитывает VWAP/VAL/VAH по последним пяти точкам данных.
func Calculate(prices, volumes []float64) (metrics.RangeMetrics, error) {
	if len(prices) < 5 || len(volumes) < 5 {
		return metrics.RangeMetrics{}, errors.New("необходимо минимум 5 наблюдений для расчёта VWAP_VA_5")
	}

	start := len(prices) - 5
	return metrics.CalculateRange(prices[start:], volumes[start:])
}
