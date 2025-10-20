package vwapvatoday

import (
	"errors"

	"invest_intraday/internal/a_submodule/variable/metrics"
)

// Calculate рассчитывает VWAP/VAL/VAH за весь торговый день.
func Calculate(prices, volumes []float64) (metrics.RangeMetrics, error) {
	if len(prices) == 0 || len(volumes) == 0 {
		return metrics.RangeMetrics{}, errors.New("для VWAP_VA_today необходимы цены и объёмы")
	}
	return metrics.CalculateRange(prices, volumes)
}
