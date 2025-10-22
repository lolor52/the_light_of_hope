package indicators

import (
	"errors"
	"math"
)

const (
	volatilityEpsilon = 1e-9
	ATRHistoryDays    = 60
	RvolHistoryDays   = 20
	atrPeriod         = 14
)

type SessionVolatilityInput struct {
	Series    SessionSeries
	PrevClose float64
}

type VolatilityMetrics struct {
	Valid bool
	Value float64
}

type volatilityHistoryDay struct {
	atrPercent []float64
	cumVolume  []float64
}

type quantilePair struct {
	P5  float64
	P95 float64
}

func CalculateVolatility(current SessionVolatilityInput, atrHistory, rvolHistory []SessionVolatilityInput) (VolatilityMetrics, error) {
	if current.Series.Length() == 0 {
		return VolatilityMetrics{}, errors.New("empty series for volatility")
	}

	currentDay, err := buildVolatilityDay(current)
	if err != nil {
		return VolatilityMetrics{}, err
	}

	atrHistDays, err := buildVolatilityHistory(atrHistory)
	if err != nil {
		return VolatilityMetrics{}, err
	}
	rvolHistDays, err := buildVolatilityHistory(rvolHistory)
	if err != nil {
		return VolatilityMetrics{}, err
	}

	atrSeries := computeATRSeries(atrHistDays)
	atrQuantiles := quantilesFromSeries(atrSeries)

	averages := computeCumVolumeAverages(rvolHistDays)
	rvolSeries := computeRVOLSeries(rvolHistDays, averages)
	rvolQuantiles := quantilesFromSeries(rvolSeries)

	sessionValues := make([]float64, len(currentDay.atrPercent))
	for i := range currentDay.atrPercent {
		atrValue := currentDay.atrPercent[i]
		avgVolume := averages[i]
		cumVolume := currentDay.cumVolume[i]

		rvolValue := math.NaN()
		if avgVolume > 0 {
			rvolValue = cumVolume / avgVolume
		}

		atrNorm := normalizeByQuantiles(atrValue, atrQuantiles[i], volatilityEpsilon)
		rvolNorm := normalizeByQuantiles(rvolValue, rvolQuantiles[i], volatilityEpsilon)

		atrNorm = safeValue(atrNorm)
		rvolNorm = safeValue(rvolNorm)

		sessionValues[i] = 100 * (0.6*atrNorm + 0.4*rvolNorm)
	}

	return VolatilityMetrics{Valid: true, Value: Mean(sessionValues)}, nil
}

func buildVolatilityHistory(inputs []SessionVolatilityInput) ([]volatilityHistoryDay, error) {
	result := make([]volatilityHistoryDay, 0, len(inputs))
	for _, input := range inputs {
		day, err := buildVolatilityDay(input)
		if err != nil {
			return nil, err
		}
		result = append(result, day)
	}
	return result, nil
}

func buildVolatilityDay(input SessionVolatilityInput) (volatilityHistoryDay, error) {
	bars := input.Series.Bars
	if len(bars) == 0 {
		return volatilityHistoryDay{}, errors.New("empty session series")
	}

	atrPercent := make([]float64, len(bars))
	cumVolume := make([]float64, len(bars))
	trValues := make([]float64, len(bars))

	prevClose := input.PrevClose
	for i, bar := range bars {
		open := bar.Open
		if open <= 0 {
			open = bar.Close
		}
		if open <= 0 {
			open = prevClose
		}
		if open <= 0 {
			open = bar.High
		}
		if open <= 0 {
			open = bar.Low
		}

		var tr float64
		if i == 0 {
			tr = math.Max(bar.High-bar.Low, math.Max(math.Abs(bar.High-open), math.Abs(bar.Low-open)))
		} else {
			tr = math.Max(bar.High-bar.Low, math.Max(math.Abs(bar.High-prevClose), math.Abs(bar.Low-prevClose)))
		}
		trValues[i] = tr

		windowStart := 0
		if i+1 > atrPeriod {
			windowStart = i + 1 - atrPeriod
		}
		windowSum := 0.0
		for j := windowStart; j <= i; j++ {
			windowSum += trValues[j]
		}
		windowLen := i - windowStart + 1
		atr := windowSum / float64(windowLen)

		closePrice := bar.Close
		if closePrice <= 0 {
			closePrice = prevClose
		}
		if closePrice > 0 {
			atrPercent[i] = atr / (closePrice + volatilityEpsilon) * 100
		} else {
			atrPercent[i] = math.NaN()
		}

		if i == 0 {
			cumVolume[i] = bar.Volume
		} else {
			cumVolume[i] = cumVolume[i-1] + bar.Volume
		}

		if bar.Close > 0 {
			prevClose = bar.Close
		}
	}

	return volatilityHistoryDay{atrPercent: atrPercent, cumVolume: cumVolume}, nil
}

func computeATRSeries(history []volatilityHistoryDay) map[int][]float64 {
	result := make(map[int][]float64)
	for _, day := range history {
		for idx, value := range day.atrPercent {
			if math.IsNaN(value) {
				continue
			}
			result[idx] = append(result[idx], value)
		}
	}
	return result
}

func computeCumVolumeAverages(history []volatilityHistoryDay) map[int]float64 {
	sums := make(map[int]float64)
	counts := make(map[int]int)
	for _, day := range history {
		for idx, value := range day.cumVolume {
			sums[idx] += value
			counts[idx]++
		}
	}
	averages := make(map[int]float64)
	for idx, sum := range sums {
		if count := counts[idx]; count > 0 {
			averages[idx] = sum / float64(count)
		}
	}
	return averages
}

func computeRVOLSeries(history []volatilityHistoryDay, averages map[int]float64) map[int][]float64 {
	result := make(map[int][]float64)
	for _, day := range history {
		for idx, value := range day.cumVolume {
			avg := averages[idx]
			if avg <= 0 {
				continue
			}
			result[idx] = append(result[idx], value/avg)
		}
	}
	return result
}

func quantilesFromSeries(series map[int][]float64) map[int]quantilePair {
	result := make(map[int]quantilePair)
	for idx, values := range series {
		clean := FilterFinite(values)
		if len(clean) == 0 {
			result[idx] = quantilePair{P5: math.NaN(), P95: math.NaN()}
			continue
		}
		result[idx] = quantilePair{
			P5:  Quantile(clean, 0.05),
			P95: Quantile(clean, 0.95),
		}
	}
	return result
}

func normalizeByQuantiles(value float64, pair quantilePair, eps float64) float64 {
	if math.IsNaN(value) {
		return math.NaN()
	}
	if math.IsNaN(pair.P5) || math.IsNaN(pair.P95) {
		return math.NaN()
	}
	if pair.P95 == pair.P5 {
		return 0.5
	}
	return Clip((value-pair.P5)/(pair.P95-pair.P5+eps), 0, 1)
}

func safeValue(value float64) float64 {
	if math.IsNaN(value) {
		return 0.5
	}
	return value
}
