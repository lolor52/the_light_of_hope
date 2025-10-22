package indicators

import (
	"fmt"
	"math"
)

// FlatTrendParams содержит параметры расчёта «плоского» тренд-фильтра.
type FlatTrendParams struct {
	// BandWidthFactor соответствует параметру b в документации.
	BandWidthFactor float64
}

// CalculateFlatTrendScore рассчитывает итоговый скор S по формуле из docs/flat_trend_filter.md.
func CalculateFlatTrendScore(series SessionSeries, params FlatTrendParams) (float64, error) {
	if series.Length() == 0 {
		return 0, fmt.Errorf("flat trend: empty series")
	}
	if params.BandWidthFactor <= 0 {
		return 0, fmt.Errorf("flat trend: invalid band width factor")
	}

	vwap := make([]float64, len(series.Bars))
	var cumVolume float64
	var cumValue float64
	for i, bar := range series.Bars {
		volume := bar.Volume
		if volume < 0 {
			volume = 0
		}
		typical := (bar.High + bar.Low + bar.Close) / 3
		cumVolume += volume
		cumValue += volume * typical
		denom := cumVolume
		if denom < machineEpsilon {
			denom = machineEpsilon
		}
		vwap[i] = cumValue / denom
	}

	rangeHigh, rangeLow := series.SessionExtremes()
	if math.IsNaN(rangeHigh) || math.IsNaN(rangeLow) {
		return 0, fmt.Errorf("flat trend: unable to determine session range")
	}
	sessionRange := rangeHigh - rangeLow
	if sessionRange < 0 {
		sessionRange = 0
	}
	if sessionRange < machineEpsilon {
		sessionRange = machineEpsilon
	}

	// Компонент 1: пересечения VWAP.
	var crossings float64
	lastSign := 0
	for i, bar := range series.Bars {
		diff := bar.Close - vwap[i]
		currentSign := sign(diff)
		if currentSign == 0 {
			continue
		}
		if lastSign != 0 && currentSign != lastSign {
			crossings++
		}
		lastSign = currentSign
	}
	tMin := float64(series.Length())
	kRef := math.Floor(tMin / 45)
	if kRef < 1 {
		kRef = 1
	}
	sCross := 100 * math.Min(1, crossings/math.Max(1, kRef))

	// Компонент 2: доля времени у VWAP.
	threshold := params.BandWidthFactor * sessionRange
	var insideCount float64
	for i, bar := range series.Bars {
		if math.Abs(bar.Close-vwap[i]) <= threshold {
			insideCount++
		}
	}
	sBand := 100 * insideCount / tMin

	// Компонент 3: сессионный дрейф.
	firstClose := series.Bars[0].Close
	lastClose := series.Bars[len(series.Bars)-1].Close
	drift := math.Abs(lastClose-firstClose) / sessionRange
	if drift > 1 {
		drift = 1
	}
	sDrift := 100 * (1 - drift)

	score := 0.4*sCross + 0.4*sBand + 0.2*sDrift
	return clip(score, 0, 100), nil
}
