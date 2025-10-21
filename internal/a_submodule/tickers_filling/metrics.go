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

type flatTrendComponents struct {
	OverlapPercent float64
	SlopeAbs       float64
	SkewPercent    float64
}

type volatilityComponents struct {
	ATRPercent     float64
	RelativeVolume float64
}

type liquidityComponents struct {
	SpreadRelativePercent float64
	TurnoverPerMinute     float64
	DepthTopFive          float64
	TickPercent           float64
}

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

func calculateFlatTrendComponents(candles []moex.MinuteCandle, current, prev moex.HistoryRow) (flatTrendComponents, error) {
	if len(candles) == 0 {
		return flatTrendComponents{}, errors.New("empty candles for flat trend filter")
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
	slopeAbs := 0.0
	if lastIndex-k >= 0 {
		vwapPrev := cumulativeValue[lastIndex-k] / (cumulativeVolume[lastIndex-k] + epsilon)
		slopeAbs = math.Abs((vwapLast - vwapPrev) / (vwapPrev + epsilon) * 100)
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

	return flatTrendComponents{
		OverlapPercent: overlapPercent,
		SlopeAbs:       slopeAbs,
		SkewPercent:    skewPercent,
	}, nil
}

func calculateVolatilityComponents(currentDay moex.HistoryRow, candles []moex.MinuteCandle, history []moex.HistoryRow) (volatilityComponents, error) {
	if len(candles) == 0 {
		return volatilityComponents{}, errors.New("empty candles for volatility")
	}
	if len(history) == 0 {
		return volatilityComponents{}, errors.New("empty history for volatility")
	}

	sort.Slice(history, func(i, j int) bool {
		return history[i].TradeDate.Before(history[j].TradeDate)
	})

	trueRanges := make([]float64, 0, len(candles))
	prevClose := candles[0].Open

	// Попробуем заменить начальное значение на закрытие предыдущего дня, если оно есть в истории.
	for i := len(history) - 1; i >= 0; i-- {
		row := history[i]
		if row.TradeDate.Before(currentDay.TradeDate) {
			prevClose = row.Close
			break
		}
	}

	for _, candle := range candles {
		rangeHighLow := candle.High - candle.Low
		rangeHighPrev := math.Abs(candle.High - prevClose)
		rangeLowPrev := math.Abs(candle.Low - prevClose)
		trueRanges = append(trueRanges, math.Max(rangeHighLow, math.Max(rangeHighPrev, rangeLowPrev)))
		prevClose = candle.Close
	}

	period := atrPeriod
	if len(trueRanges) < period {
		period = len(trueRanges)
	}
	if period == 0 {
		return volatilityComponents{}, errors.New("not enough data for ATR")
	}

	var atrSum float64
	for _, value := range trueRanges[len(trueRanges)-period:] {
		atrSum += value
	}
	atr := atrSum / float64(period)

	closePrice := candles[len(candles)-1].Close
	if closePrice <= 0 {
		closePrice = currentDay.Close
	}

	atrPercent := 0.0
	if closePrice > 0 {
		atrPercent = atr / (closePrice + epsilon) * 100
	}

	volumes := make([]float64, 0, len(history))
	for _, row := range history {
		if row.TradeDate.Before(currentDay.TradeDate) {
			volumes = append(volumes, row.Volume)
		}
	}

	if len(volumes) > relativeVolumePeriod {
		volumes = volumes[len(volumes)-relativeVolumePeriod:]
	}

	var avgVolume float64
	if len(volumes) > 0 {
		for _, volume := range volumes {
			avgVolume += volume
		}
		avgVolume /= float64(len(volumes))
	}

	rvol := 0.0
	if avgVolume > 0 {
		rvol = currentDay.Volume / avgVolume
	}

	return volatilityComponents{
		ATRPercent:     atrPercent,
		RelativeVolume: rvol,
	}, nil
}

func calculateLiquidityComponents(candles []moex.MinuteCandle, marketData moex.MarketData, book moex.OrderBook, info moex.SecurityInfo) (liquidityComponents, error) {
	if len(candles) == 0 {
		return liquidityComponents{}, errors.New("empty candles for liquidity")
	}

	mid := (marketData.Bid + marketData.Offer) / 2
	if mid <= 0 {
		mid = marketData.Last
	}
	if mid <= 0 {
		return liquidityComponents{}, errors.New("invalid mid price for liquidity")
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

	return liquidityComponents{
		SpreadRelativePercent: spreadRel,
		TurnoverPerMinute:     turnoverPerMinute,
		DepthTopFive:          depthTotal,
		TickPercent:           tickPercent,
	}, nil
}
