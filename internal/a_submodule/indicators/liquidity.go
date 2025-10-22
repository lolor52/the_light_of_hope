package indicators

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

type LiquidityMetrics struct {
	Valid           bool
	VTotal          float64
	VMedian         float64
	ActiveShare     float64
	Illiq           float64
	Roll            float64
	RollFromCandles bool
	TickPercent     float64
	DepthProxy      float64
}

func CalculateLiquidity(series SessionSeries, trades []moex.Trade, info moex.SecurityInfo) (LiquidityMetrics, error) {
	if series.Length() < minMinutesForSession {
		return LiquidityMetrics{Valid: false}, nil
	}

	metrics := LiquidityMetrics{Valid: true}

	metrics.VTotal = series.TotalValue()

	activeValues := series.ActiveValues(func(bar MinuteBar) (float64, bool) {
		return bar.Value, true
	})
	if len(activeValues) > 0 {
		metrics.VMedian = Median(activeValues)
	} else {
		metrics.VMedian = math.NaN()
	}

	metrics.ActiveShare = float64(series.ActiveCount()) / float64(series.Length())

	logs := series.LogReturns()
	values := series.Values()
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
		metrics.Illiq = Mean(illiqValues)
	} else {
		metrics.Illiq = math.NaN()
	}

	roll, fromCandles := calculateRoll(series, trades)
	metrics.Roll = roll
	metrics.RollFromCandles = fromCandles

	priceRef := series.PriceRef()
	if info.MinStep > 0 && priceRef > 0 {
		metrics.TickPercent = 10000 * info.MinStep / priceRef
	} else {
		metrics.TickPercent = math.NaN()
	}

	metrics.DepthProxy = calculateDepthProxy(series)

	return metrics, nil
}

func calculateRoll(series SessionSeries, trades []moex.Trade) (float64, bool) {
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
	closes := series.Closes()
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

func filterTrades(series SessionSeries, trades []moex.Trade) []moex.Trade {
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

func seriesBounds(series SessionSeries) sessionBounds {
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
	meanX := Mean(x)
	meanY := Mean(y)
	var sum float64
	for i := range y {
		sum += (x[i] - meanX) * (y[i] - meanY)
	}
	return sum / float64(len(y))
}

func calculateDepthProxy(series SessionSeries) float64 {
	returns := series.LogReturns()
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
	tau := Quantile(rValues, 0.05)
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
	return Median(valueOverReturn)
}

func NormalizeLiquidity(items []LiquidityMetrics) []float64 {
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
		if !item.Valid {
			continue
		}
		values["vtotal"] = append(values["vtotal"], item.VTotal)
		values["vmedian"] = append(values["vmedian"], item.VMedian)
		values["depth"] = append(values["depth"], item.DepthProxy)
		values["active"] = append(values["active"], item.ActiveShare)
		values["illiq"] = append(values["illiq"], item.Illiq)
		values["roll"] = append(values["roll"], item.Roll)
		values["tick"] = append(values["tick"], item.TickPercent)
	}

	quantiles := make(map[string]struct{ p5, p95 float64 })
	for key, arr := range values {
		clean := FilterFinite(arr)
		if len(clean) == 0 {
			quantiles[key] = struct{ p5, p95 float64 }{math.NaN(), math.NaN()}
			continue
		}
		quantiles[key] = struct{ p5, p95 float64 }{
			p5:  Quantile(clean, 0.05),
			p95: Quantile(clean, 0.95),
		}
	}

	scores := make([]float64, len(items))
	for i, item := range items {
		if !item.Valid {
			scores[i] = math.NaN()
			continue
		}
		vTotalNorm := normUpQuantile(item.VTotal, quantiles["vtotal"])
		vMedianNorm := normUpQuantile(item.VMedian, quantiles["vmedian"])
		depthNorm := normUpQuantile(item.DepthProxy, quantiles["depth"])
		activeNorm := normUpQuantile(item.ActiveShare, quantiles["active"])
		illiqNorm := normDownQuantile(item.Illiq, quantiles["illiq"])
		rollNorm := normDownQuantile(item.Roll, quantiles["roll"])
		tickNorm := normDownQuantile(item.TickPercent, quantiles["tick"])

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

		scores[i] = 100 * Clip(score, 0, 1)
	}

	return scores
}

func normUpQuantile(value float64, q struct{ p5, p95 float64 }) float64 {
	if math.IsNaN(value) {
		return math.NaN()
	}
	if math.IsNaN(q.p5) || math.IsNaN(q.p95) || q.p5 == q.p95 {
		return math.NaN()
	}
	return Clip((value-q.p5)/(q.p95-q.p5+liquidityEpsilon), 0, 1)
}

func normDownQuantile(value float64, q struct{ p5, p95 float64 }) float64 {
	up := normUpQuantile(value, q)
	if math.IsNaN(up) {
		return math.NaN()
	}
	return 1 - up
}
