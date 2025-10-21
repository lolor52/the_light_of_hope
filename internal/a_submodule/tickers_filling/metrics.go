package tickers_filling

import (
	"errors"
	"math"
	"sort"

	"invest_intraday/internal/a_submodule/moex"
)

const (
	epsilon                = 1e-9
	vwapSlopeWindow        = 10
	atrPeriod              = 14
	relativeVolumePeriod   = 20
	valueAreaCoverageRatio = 0.7
)

func calculateVWAP(candles []moex.MinuteCandle) (float64, error) {
	var totalValue, totalVolume float64
	for _, candle := range candles {
		totalValue += candle.Value
		totalVolume += candle.Volume
	}
	if totalVolume <= 0 {
		return 0, errors.New("zero volume for VWAP")
	}
	return totalValue / totalVolume, nil
}

type valueAreaResult struct {
	VAL float64
	VAH float64
}

func calculateValueArea(candles []moex.MinuteCandle, minStep float64) (valueAreaResult, error) {
	if len(candles) == 0 {
		return valueAreaResult{}, errors.New("empty candles for value area")
	}

	step := minStep
	if step <= 0 {
		step = 0.01
	}

	volumes := make(map[float64]float64)
	var totalVolume float64
	for _, candle := range candles {
		price := candle.Close
		if price == 0 {
			price = (candle.High + candle.Low) / 2
		}
		if price == 0 {
			continue
		}
		bucket := math.Round(price/step) * step
		volumes[bucket] += candle.Volume
		totalVolume += candle.Volume
	}
	if totalVolume <= 0 {
		return valueAreaResult{}, errors.New("no volume in value area calculation")
	}

	type priceVolume struct {
		Price  float64
		Volume float64
	}

	buckets := make([]priceVolume, 0, len(volumes))
	for price, volume := range volumes {
		buckets = append(buckets, priceVolume{Price: price, Volume: volume})
	}

	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Volume == buckets[j].Volume {
			return buckets[i].Price < buckets[j].Price
		}
		return buckets[i].Volume > buckets[j].Volume
	})

	if len(buckets) == 0 {
		return valueAreaResult{}, errors.New("no buckets for value area")
	}

	// Найдём максимальный объём и центр распределения.
	maxBucket := buckets[0]

	// Для расширения в обе стороны отсортируем цены по возрастанию.
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Price < buckets[j].Price
	})

	// Найдём индекс максимального объёма.
	centerIndex := 0
	for i, bucket := range buckets {
		if bucket.Price == maxBucket.Price {
			centerIndex = i
			break
		}
	}

	targetVolume := totalVolume * valueAreaCoverageRatio
	accumulated := buckets[centerIndex].Volume
	lowerIndex := centerIndex
	upperIndex := centerIndex

	for accumulated < targetVolume {
		var nextLowerVolume float64 = -1
		var nextUpperVolume float64 = -1
		if lowerIndex > 0 {
			nextLowerVolume = buckets[lowerIndex-1].Volume
		}
		if upperIndex < len(buckets)-1 {
			nextUpperVolume = buckets[upperIndex+1].Volume
		}

		if nextLowerVolume < 0 && nextUpperVolume < 0 {
			break
		}

		if nextUpperVolume >= nextLowerVolume {
			upperIndex++
			accumulated += buckets[upperIndex].Volume
		} else {
			lowerIndex--
			accumulated += buckets[lowerIndex].Volume
		}
	}

	result := valueAreaResult{
		VAL: buckets[lowerIndex].Price,
		VAH: buckets[upperIndex].Price,
	}

	return result, nil
}

func calculateFlatTrendFilter(candles []moex.MinuteCandle, current, prev moex.HistoryRow) (float64, error) {
	if len(candles) == 0 {
		return 0, errors.New("empty candles for flat trend filter")
	}

	cumulativeValue := make([]float64, len(candles))
	cumulativeVolume := make([]float64, len(candles))
	for i, candle := range candles {
		cumulativeValue[i] = candle.Value
		cumulativeVolume[i] = candle.Volume
		if i > 0 {
			cumulativeValue[i] += cumulativeValue[i-1]
			cumulativeVolume[i] += cumulativeVolume[i-1]
		}
	}

	lastIndex := len(candles) - 1
	vwapLast := cumulativeValue[lastIndex] / (cumulativeVolume[lastIndex] + epsilon)
	k := vwapSlopeWindow
	var slope float64
	if lastIndex-k >= 0 {
		vwapPrev := cumulativeValue[lastIndex-k] / (cumulativeVolume[lastIndex-k] + epsilon)
		if vwapPrev != 0 {
			slope = (vwapLast - vwapPrev) / (vwapPrev + epsilon) * 100
		}
	}

	overlapWidth := math.Max(0, math.Min(current.High, prev.High)-math.Max(current.Low, prev.Low))
	unionWidth := math.Max(current.High, prev.High) - math.Min(current.Low, prev.Low)
	overlapPercent := 0.0
	if unionWidth > 0 {
		overlapPercent = 100 * overlapWidth / (unionWidth + epsilon)
	}

	mid := (current.High + current.Low) / 2
	vwapDay := current.Value / (current.Volume + epsilon)
	rangeWidth := current.High - current.Low
	skewPercent := 0.0
	if rangeWidth > 0 {
		skewPercent = math.Abs(vwapDay-mid) / (rangeWidth + epsilon) * 100
	}

	overlapNorm := clamp(overlapPercent/100, 0, 1)
	slopeNorm := clamp(math.Abs(slope)/10, 0, 1)
	skewNorm := clamp(skewPercent/100, 0, 1)

	filter := 100 * (0.5*overlapNorm + 0.3*(1-slopeNorm) + 0.2*(1-skewNorm))

	return filter, nil
}

func calculateVolatility(currentDate moex.HistoryRow, history []moex.HistoryRow) (float64, error) {
	if len(history) == 0 {
		return 0, errors.New("empty history for volatility")
	}

	sort.Slice(history, func(i, j int) bool {
		return history[i].TradeDate.Before(history[j].TradeDate)
	})

	// Составим TR по истории.
	trueRanges := make([]float64, len(history))
	for i, row := range history {
		prevClose := row.Close
		if i > 0 {
			prevClose = history[i-1].Close
		}
		rangeHighLow := row.High - row.Low
		rangeHighPrev := math.Abs(row.High - prevClose)
		rangeLowPrev := math.Abs(row.Low - prevClose)
		trueRanges[i] = math.Max(rangeHighLow, math.Max(rangeHighPrev, rangeLowPrev))
	}

	// Найдём индекс текущей даты.
	currentIndex := -1
	for i, row := range history {
		if row.TradeDate.Equal(currentDate.TradeDate) {
			currentIndex = i
			break
		}
	}
	if currentIndex == -1 {
		return 0, errors.New("current date not found in history for volatility")
	}

	start := currentIndex - atrPeriod + 1
	if start < 0 {
		start = 0
	}
	var sumATR float64
	for i := start; i <= currentIndex; i++ {
		sumATR += trueRanges[i]
	}
	atr := sumATR / float64(currentIndex-start+1)

	atrPercent := 0.0
	if currentDate.Close > 0 {
		atrPercent = atr / (currentDate.Close + epsilon) * 100
	}

	startVol := currentIndex - relativeVolumePeriod + 1
	if startVol < 0 {
		startVol = 0
	}
	var sumVolume float64
	for i := startVol; i <= currentIndex; i++ {
		sumVolume += history[i].Volume
	}
	avgVolume := sumVolume / float64(currentIndex-startVol+1)
	rvol := 0.0
	if avgVolume > 0 {
		rvol = currentDate.Volume / avgVolume
	}

	atrNorm := clamp(atrPercent/15, 0, 1)
	rvolNorm := clamp(rvol/3, 0, 1)

	volatility := 100 * (0.6*atrNorm + 0.4*rvolNorm)

	return volatility, nil
}

func calculateLiquidity(candles []moex.MinuteCandle, marketData moex.MarketData, book moex.OrderBook, info moex.SecurityInfo) (float64, error) {
	if len(candles) == 0 {
		return 0, errors.New("empty candles for liquidity")
	}

	mid := (marketData.Bid + marketData.Offer) / 2
	if mid <= 0 {
		mid = marketData.Last
	}
	if mid <= 0 {
		return 0, errors.New("invalid mid price for liquidity")
	}

	spreadAbs := marketData.Offer - marketData.Bid
	if spreadAbs < 0 {
		spreadAbs = 0
	}
	spreadRel := spreadAbs / (mid + epsilon) * 100

	var totalValue float64
	for _, candle := range candles {
		totalValue += candle.Value
	}
	turnoverPerMinute := totalValue / float64(len(candles))

	var depthBid float64
	for i := 0; i < len(book.Bids) && i < 5; i++ {
		depthBid += book.Bids[i].Quantity
	}
	var depthAsk float64
	for i := 0; i < len(book.Asks) && i < 5; i++ {
		depthAsk += book.Asks[i].Quantity
	}
	lotSize := info.LotSize
	if lotSize <= 0 {
		lotSize = 1
	}
	depthTotal := (depthBid + depthAsk) * lotSize

	tickPercent := 0.0
	if info.MinStep > 0 {
		tickPercent = info.MinStep / (mid + epsilon) * 100
	}

	spreadComponent := 1 - clamp(spreadRel/1, 0, 1)
	turnoverComponent := clamp(turnoverPerMinute/5e7, 0, 1)
	depthComponent := clamp(depthTotal/1e7, 0, 1)
	tickComponent := 1 - clamp(tickPercent/0.5, 0, 1)

	liquidity := 100 * (0.35*spreadComponent + 0.35*turnoverComponent + 0.25*depthComponent + 0.05*tickComponent)

	return liquidity, nil
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
