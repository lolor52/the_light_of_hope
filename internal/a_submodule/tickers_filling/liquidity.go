package tickers_filling

import (
	"math"
	"sort"
	"time"

	"invest_intraday/internal/a_submodule/moex"
)

const (
	liquidityEpsilon     = 1e-12
	minMinutesForSession = 60
	minTradesForRoll     = 3
)

type liquidityMetrics struct {
	valid           bool
	vTotal          float64
	vMedian         float64
	activeShare     float64
	illiq           float64
	roll            float64
	rollFromCandles bool
	tickPercent     float64
	depthProxy      float64
}

func calculateLiquidity(series mainSessionSeries, trades []moex.Trade, info moex.SecurityInfo) (liquidityMetrics, error) {
	if series.length() < minMinutesForSession {
		return liquidityMetrics{valid: false}, nil
	}

	metrics := liquidityMetrics{valid: true}

	metrics.vTotal = series.totalValue()

	activeValues := series.activeValues(func(bar minuteBar) (float64, bool) {
		return bar.Value, true
	})
	if len(activeValues) > 0 {
		metrics.vMedian = median(activeValues)
	} else {
		metrics.vMedian = math.NaN()
	}

	metrics.activeShare = float64(series.activeCount()) / float64(series.length())

	logs := series.logReturns()
	values := series.values()
	illiqValues := make([]float64, 0, len(series.Bars))
	for i := range series.Bars {
		if !series.Bars[i].Active {
			continue
		}
		if i == 0 {
			continue
		}
		r := logs[i]
		if math.IsNaN(r) {
			continue
		}
		denom := values[i] + liquidityEpsilon
		illiqValues = append(illiqValues, r/denom)
	}
	if len(illiqValues) > 0 {
		metrics.illiq = mean(illiqValues)
	} else {
		metrics.illiq = math.NaN()
	}

	roll, fromCandles := calculateRoll(series, trades)
	metrics.roll = roll
	metrics.rollFromCandles = fromCandles

	priceRef := series.priceRef()
	if info.MinStep > 0 && priceRef > 0 {
		metrics.tickPercent = 10000 * info.MinStep / priceRef
	} else {
		metrics.tickPercent = math.NaN()
	}

	metrics.depthProxy = calculateDepthProxy(series)

	return metrics, nil
}

func calculateRoll(series mainSessionSeries, trades []moex.Trade) (float64, bool) {
	filtered := filterTrades(series, trades)
	if len(filtered) >= minTradesForRoll {
		diffs := tradeDiffs(filtered)
		if len(diffs) >= 2 {
			cov := laggedCovariance(diffs)
			if cov < 0 {
				return 2 * math.Sqrt(math.Max(0, -cov)), false
			}
		}
	}

	diffs := make([]float64, 0, len(series.Bars))
	closes := series.closes()
	for i := 1; i < len(closes); i++ {
		if closes[i] == 0 || closes[i-1] == 0 {
			continue
		}
		diffs = append(diffs, closes[i]-closes[i-1])
	}
	if len(diffs) < 2 {
		return math.NaN(), true
	}
	cov := laggedCovariance(diffs)
	if cov >= 0 {
		return math.NaN(), true
	}
	return 2 * math.Sqrt(math.Max(0, -cov)), true
}

func filterTrades(series mainSessionSeries, trades []moex.Trade) []moex.Trade {
	bounds := seriesBounds(series)
	result := make([]moex.Trade, 0, len(trades))
	for _, trade := range trades {
		if trade.Price <= 0 {
			continue
		}
		if !trade.Time.Before(bounds.end) || trade.Time.Before(bounds.start) {
			continue
		}
		result = append(result, trade)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Before(result[j].Time)
	})
	return result
}

type sessionBounds struct {
	start time.Time
	end   time.Time
}

func seriesBounds(series mainSessionSeries) sessionBounds {
	if len(series.Bars) == 0 {
		return sessionBounds{}
	}
	return sessionBounds{start: series.Bars[0].Time, end: series.Bars[len(series.Bars)-1].Time.Add(time.Minute)}
}

func tradeDiffs(trades []moex.Trade) []float64 {
	diffs := make([]float64, 0, len(trades)-1)
	for i := 1; i < len(trades); i++ {
		diffs = append(diffs, trades[i].Price-trades[i-1].Price)
	}
	return diffs
}

func laggedCovariance(diffs []float64) float64 {
	if len(diffs) < 2 {
		return math.NaN()
	}
	x := diffs[1:]
	y := diffs[:len(diffs)-1]
	meanX := mean(x)
	meanY := mean(y)
	var sum float64
	for i := range y {
		sum += (x[i] - meanX) * (y[i] - meanY)
	}
	return sum / float64(len(y))
}

func calculateDepthProxy(series mainSessionSeries) float64 {
	returns := series.logReturns()
	rValues := make([]float64, 0, len(series.Bars))
	valueOverReturn := make([]float64, 0, len(series.Bars))
	for i := range series.Bars {
		if !series.Bars[i].Active {
			continue
		}
		if math.IsNaN(returns[i]) {
			continue
		}
		rValues = append(rValues, returns[i])
	}
	if len(rValues) == 0 {
		return math.NaN()
	}
	tau := quantile(rValues, 0.05)
	for i := range series.Bars {
		if !series.Bars[i].Active {
			continue
		}
		if math.IsNaN(returns[i]) {
			continue
		}
		denom := math.Max(returns[i], tau)
		if denom <= 0 {
			continue
		}
		valueOverReturn = append(valueOverReturn, series.Bars[i].Value/denom)
	}
	if len(valueOverReturn) == 0 {
		return math.NaN()
	}
	return median(valueOverReturn)
}

func normalizeLiquidity(items []liquidityMetrics) []float64 {
	if len(items) == 0 {
		return nil
	}

	values := map[string][]float64{
		"vtotal":  make([]float64, 0, len(items)),
		"vmedian": make([]float64, 0, len(items)),
		"depth":   make([]float64, 0, len(items)),
		"active":  make([]float64, 0, len(items)),
		"illiq":   make([]float64, 0, len(items)),
		"roll":    make([]float64, 0, len(items)),
		"tick":    make([]float64, 0, len(items)),
	}

	for _, item := range items {
		if !item.valid {
			continue
		}
		values["vtotal"] = append(values["vtotal"], item.vTotal)
		values["vmedian"] = append(values["vmedian"], item.vMedian)
		values["depth"] = append(values["depth"], item.depthProxy)
		values["active"] = append(values["active"], item.activeShare)
		values["illiq"] = append(values["illiq"], item.illiq)
		values["roll"] = append(values["roll"], item.roll)
		values["tick"] = append(values["tick"], item.tickPercent)
	}

	quantiles := make(map[string]struct{ p5, p95 float64 })
	for key, arr := range values {
		clean := filterFinite(arr)
		if len(clean) == 0 {
			quantiles[key] = struct{ p5, p95 float64 }{math.NaN(), math.NaN()}
			continue
		}
		quantiles[key] = struct{ p5, p95 float64 }{
			p5:  quantile(clean, 0.05),
			p95: quantile(clean, 0.95),
		}
	}

	scores := make([]float64, len(items))
	for i, item := range items {
		if !item.valid {
			scores[i] = math.NaN()
			continue
		}
		vTotalNorm := normUpQuantile(item.vTotal, quantiles["vtotal"])
		vMedianNorm := normUpQuantile(item.vMedian, quantiles["vmedian"])
		depthNorm := normUpQuantile(item.depthProxy, quantiles["depth"])
		activeNorm := normUpQuantile(item.activeShare, quantiles["active"])
		illiqNorm := normDownQuantile(item.illiq, quantiles["illiq"])
		rollNorm := normDownQuantile(item.roll, quantiles["roll"])
		tickNorm := normDownQuantile(item.tickPercent, quantiles["tick"])

		vTotalNorm = safeValue(vTotalNorm)
		vMedianNorm = safeValue(vMedianNorm)
		depthNorm = safeValue(depthNorm)
		activeNorm = safeValue(activeNorm)
		illiqNorm = safeValue(illiqNorm)
		rollNorm = safeValue(rollNorm)
		tickNorm = safeValue(tickNorm)

		score := 0.0
		score += 0.2 * vTotalNorm
		score += 0.2 * vMedianNorm
		score += 0.2 * depthNorm
		score += 0.15 * activeNorm
		score += 0.15 * illiqNorm
		score += 0.05 * rollNorm
		score += 0.05 * tickNorm

		scores[i] = 100 * clip(score, 0, 1)
	}

	return scores
}

func filterFinite(values []float64) []float64 {
	result := make([]float64, 0, len(values))
	for _, value := range values {
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			result = append(result, value)
		}
	}
	return result
}

func normUpQuantile(value float64, q struct{ p5, p95 float64 }) float64 {
	if math.IsNaN(value) {
		return math.NaN()
	}
	if math.IsNaN(q.p5) || math.IsNaN(q.p95) || q.p5 == q.p95 {
		return math.NaN()
	}
	return clip((value-q.p5)/(q.p95-q.p5+liquidityEpsilon), 0, 1)
}

func normDownQuantile(value float64, q struct{ p5, p95 float64 }) float64 {
	up := normUpQuantile(value, q)
	if math.IsNaN(up) {
		return math.NaN()
	}
	return 1 - up
}
