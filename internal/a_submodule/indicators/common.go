package indicators

import (
	"math"
	"sort"
	"time"
)

const (
	// machineEpsilon защищает от деления на ноль в формулах индикаторов.
	machineEpsilon = 1e-9
)

// MinuteBar описывает минутный бар основной сессии.
type MinuteBar struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Value  float64
	Active bool
}

// SessionSeries содержит последовательность минутных баров основной сессии.
type SessionSeries struct {
	Bars []MinuteBar
}

// Length возвращает количество баров в серии.
func (series SessionSeries) Length() int {
	return len(series.Bars)
}

// SessionExtremes возвращает максимум и минимум цен за сессию.
func (series SessionSeries) SessionExtremes() (float64, float64) {
	high := math.Inf(-1)
	low := math.Inf(1)
	for _, bar := range series.Bars {
		if bar.High > high {
			high = bar.High
		}
		if bar.Low < low {
			low = bar.Low
		}
	}
	if math.IsInf(high, -1) {
		high = math.NaN()
	}
	if math.IsInf(low, 1) {
		low = math.NaN()
	}
	return high, low
}

// clip ограничивает значение указанным интервалом.
func clip(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

// quantile вычисляет квантиль q для набора значений.
func quantile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[len(sorted)-1]
	}

	pos := q * float64(len(sorted)-1)
	lower := int(math.Floor(pos))
	upper := int(math.Ceil(pos))
	if lower == upper {
		return sorted[lower]
	}
	weight := pos - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// filterFinite оставляет только конечные значения.
func filterFinite(values []float64) []float64 {
	result := make([]float64, 0, len(values))
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		result = append(result, value)
	}
	return result
}

// sign возвращает знак числа (-1, 0, 1).
func sign(value float64) int {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}
