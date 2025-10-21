package tickers_selection

import (
	"errors"
	"math"
)

const (
	freshnessWeightsCount = 5
	epsilon               = 1e-9
	trendThreshold        = 60.0
)

var freshnessWeights = [freshnessWeightsCount]float64{0.03, 0.07, 0.20, 0.30, 0.40}

type tickerScores struct {
	Regime             Regime
	FinalScore         float64
	MeanReversionScore float64
	MomentumScore      float64
	TrendScore         float64
	DeltaVWAPPct       float64
	OverlapPercent     float64
	Breakout           bool
}

func calculateScores(sessions []sessionMetrics) (tickerScores, error) {
	var scores tickerScores
	if len(sessions) < freshnessWeightsCount {
		return scores, ErrNotEnoughSessions
	}

	latest := sessions[len(sessions)-freshnessWeightsCount:]

	vwPrev := latest[freshnessWeightsCount-2].VWAP
	vwLast := latest[freshnessWeightsCount-1].VWAP
	deltaVWAP := (vwLast - vwPrev) / (vwPrev + epsilon) * 100

	vahPrev := latest[freshnessWeightsCount-2].VAH
	valPrev := latest[freshnessWeightsCount-2].VAL
	vahLast := latest[freshnessWeightsCount-1].VAH
	valLast := latest[freshnessWeightsCount-1].VAL

	breakout := vwLast > vahPrev || vwLast < valPrev

	overlapWidth := math.Max(0, math.Min(vahLast, vahPrev)-math.Max(valLast, valPrev))
	unionWidth := math.Max(vahLast, vahPrev) - math.Min(valLast, valPrev)
	overlapPercent := 0.0
	if unionWidth > 0 {
		overlapPercent = 100 * overlapWidth / (unionWidth + epsilon)
	}

	trendScore := 0.4*(100-latest[freshnessWeightsCount-1].FlatTrend) +
		0.3*(100-overlapPercent) +
		0.2*math.Abs(deltaVWAP) +
		0.1*(100*boolToFloat(breakout))

	regime := RegimeRange
	if trendScore >= trendThreshold {
		regime = RegimeTrend
	}

	liquidityValues := make([]float64, freshnessWeightsCount)
	volatilityValues := make([]float64, freshnessWeightsCount)
	flatValues := make([]float64, freshnessWeightsCount)

	var liquidityWeighted, volatilityWeighted, flatWeighted float64
	for i, weight := range freshnessWeights {
		liquidityValues[i] = latest[i].Liquidity
		volatilityValues[i] = latest[i].Volatility
		flatValues[i] = latest[i].FlatTrend
		liquidityWeighted += weight * latest[i].Liquidity
		volatilityWeighted += weight * latest[i].Volatility
		flatWeighted += weight * latest[i].FlatTrend
	}

	rl := max(liquidityValues) - min(liquidityValues)
	rv := max(volatilityValues) - min(volatilityValues)
	rf := max(flatValues) - min(flatValues)
	penalty := 0.3 * (rl + rv + rf) / 3

	vMid := math.Max(0, 100-2*math.Abs(volatilityWeighted-50))

	scoreMR := clamp(0.5*liquidityWeighted+0.3*flatWeighted+0.2*vMid-penalty, 0, 100)
	scoreMO := clamp(0.5*liquidityWeighted+0.35*volatilityWeighted+0.15*(100-flatWeighted)-penalty, 0, 100)

	finalScore := scoreMR
	if regime == RegimeTrend {
		finalScore = scoreMO
	}

	scores = tickerScores{
		Regime:             regime,
		FinalScore:         finalScore,
		MeanReversionScore: scoreMR,
		MomentumScore:      scoreMO,
		TrendScore:         trendScore,
		DeltaVWAPPct:       deltaVWAP,
		OverlapPercent:     overlapPercent,
		Breakout:           breakout,
	}

	return scores, nil
}

// ErrNotEnoughSessions возвращается, когда данных недостаточно для расчёта.
var ErrNotEnoughSessions = errors.New("not enough sessions")

func clamp(x, minValue, maxValue float64) float64 {
	if x < minValue {
		return minValue
	}
	if x > maxValue {
		return maxValue
	}
	return x
}

func boolToFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, value := range values[1:] {
		if value < m {
			m = value
		}
	}
	return m
}

func max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, value := range values[1:] {
		if value > m {
			m = value
		}
	}
	return m
}
