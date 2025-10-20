package vwapvares

import (
	"errors"

	"invest_intraday/internal/a_submodule/variable/metrics"
)

// Calculate рассчитывает итоговые VWAP/VAL/VAH для выбранного окна наблюдений.
func Calculate(prices, volumes []float64) (metrics.RangeMetrics, error) {
	if len(prices) == 0 || len(volumes) == 0 {
		return metrics.RangeMetrics{}, errors.New("для VWAP_VA_res необходимы цены и объёмы")
	}
	return metrics.CalculateRange(prices, volumes)
}
