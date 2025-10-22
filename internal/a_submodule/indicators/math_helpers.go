package indicators

import (
	"math"
	"sort"
)

func Sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

func Median(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return 0.5 * (sorted[mid-1] + sorted[mid])
}

func Quantile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	if q <= 0 {
		return MinFloat(values)
	}
	if q >= 1 {
		return MaxFloat(values)
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	pos := q * float64(len(sorted)-1)
	lower := int(math.Floor(pos))
	upper := int(math.Ceil(pos))
	if lower == upper {
		return sorted[lower]
	}
	weight := pos - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func Clip(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func MinFloat(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	minVal := values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
}

func MaxFloat(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	maxVal := values[0]
	for _, v := range values[1:] {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

func Mean(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	return Sum(values) / float64(len(values))
}

func FilterFinite(values []float64) []float64 {
	result := make([]float64, 0, len(values))
	for _, value := range values {
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			result = append(result, value)
		}
	}
	return result
}

func SafeValue(value float64) float64 {
	if math.IsNaN(value) {
		return 0.5
	}
	return value
}
