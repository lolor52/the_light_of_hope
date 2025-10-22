package tickers_filling

import (
	"errors"
	"math"
)

const (
	flatTrendEpsilon = 1e-9
	vwapSlopeWindow  = 10
)

type flatTrendComponents struct {
	overlapPercent float64
	slopeAbs       float64
	skewPercent    float64
}

func calculateFlatTrendComponents(currentSeries, prevSeries mainSessionSeries) (flatTrendComponents, error) {
	if currentSeries.length() == 0 {
		return flatTrendComponents{}, errors.New("empty current series for flat trend filter")
	}
	if prevSeries.length() == 0 {
		return flatTrendComponents{}, errors.New("empty previous series for flat trend filter")
	}

	values, volumes := currentSeries.cumulativeValues()
	lastIndex := len(values) - 1
	slope := math.NaN()
	if lastIndex >= vwapSlopeWindow {
		vwapLast := values[lastIndex] / (volumes[lastIndex] + flatTrendEpsilon)
		prevIdx := lastIndex - vwapSlopeWindow
		vwapPrev := values[prevIdx] / (volumes[prevIdx] + flatTrendEpsilon)
		slope = math.Abs((vwapLast - vwapPrev) / (vwapPrev + flatTrendEpsilon) * 100)
	}

	currentHigh, currentLow := currentSeries.sessionExtremes()
	prevHigh, prevLow := prevSeries.sessionExtremes()

	overlapWidth := math.Max(0, math.Min(currentHigh, prevHigh)-math.Max(currentLow, prevLow))
	unionWidth := math.Max(currentHigh, prevHigh) - math.Min(currentLow, prevLow)
	overlap := math.NaN()
	if unionWidth > 0 {
		overlap = overlapWidth / (unionWidth + flatTrendEpsilon) * 100
	}

	valueSum, volumeSum, vwap := currentSeries.dailyAggregates()
	_ = valueSum
	dayRange := currentHigh - currentLow
	skew := math.NaN()
	if dayRange > 0 {
		mid := (currentHigh + currentLow) / 2
		skew = math.Abs(vwap-mid) / (dayRange + flatTrendEpsilon) * 100
	}

	return flatTrendComponents{
		overlapPercent: overlap,
		slopeAbs:       slope,
		skewPercent:    skew,
	}, nil
}

func normalizeFlatTrend(items []flatTrendComponents) []float64 {
	if len(items) == 0 {
		return nil
	}
	overlaps := make([]float64, len(items))
	slopes := make([]float64, len(items))
	skews := make([]float64, len(items))
	for i, item := range items {
		overlaps[i] = item.overlapPercent
		slopes[i] = item.slopeAbs
		skews[i] = item.skewPercent
	}

	overlapNorm := normalizeUp(overlaps)
	slopeNorm := normalizeDown(slopes)
	skewNorm := normalizeDown(skews)

	result := make([]float64, len(items))
	for i := range items {
		ov := safeValue(overlapNorm[i])
		sl := safeValue(slopeNorm[i])
		sk := safeValue(skewNorm[i])
		result[i] = 100 * (0.5*ov + 0.3*sl + 0.2*sk)
	}
	return result
}

func safeValue(value float64) float64 {
	if math.IsNaN(value) {
		return 0.5
	}
	return value
}

func normalizeUp(values []float64) []float64 {
	result := make([]float64, len(values))
	if len(values) == 0 {
		return result
	}
	minVal, maxVal := normalizationBounds(values)
	for i, value := range values {
		if math.IsNaN(value) {
			result[i] = math.NaN()
			continue
		}
		result[i] = clip((value-minVal)/(maxVal-minVal+flatTrendEpsilon), 0, 1)
	}
	return result
}

func normalizeDown(values []float64) []float64 {
	result := make([]float64, len(values))
	if len(values) == 0 {
		return result
	}
	minVal, maxVal := normalizationBounds(values)
	for i, value := range values {
		if math.IsNaN(value) {
			result[i] = math.NaN()
			continue
		}
		up := clip((value-minVal)/(maxVal-minVal+flatTrendEpsilon), 0, 1)
		result[i] = 1 - up
	}
	return result
}

func normalizationBounds(values []float64) (float64, float64) {
	finite := make([]float64, 0, len(values))
	for _, value := range values {
		if !math.IsNaN(value) {
			finite = append(finite, value)
		}
	}
	if len(finite) == 0 {
		return 0, 0
	}
	minVal := minFloat(finite)
	maxVal := maxFloat(finite)
	if maxVal == minVal {
		if minVal == 0 {
			return 0, 1
		}
		return minVal, minVal + 1
	}
	return minVal, maxVal
}
