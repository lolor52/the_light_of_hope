package tickers_filling

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	indicators "invest_intraday/internal/a_submodule/indicators"
	"invest_intraday/internal/a_submodule/moex"
)

type sessionSchedule struct {
	MainStart         time.Time
	MainEnd           time.Time
	AuctionOpenStart  *time.Time
	AuctionOpenEnd    *time.Time
	AuctionCloseStart *time.Time
	AuctionCloseEnd   *time.Time
}

type minuteBar = indicators.MinuteBar

type mainSessionSeries = indicators.SessionSeries

type priceFallbacks struct {
	PrevSessionClose float64
	FirstTradePrice  float64
}

const valueAreaCoverageRatio = 0.7

type valueAreaResult struct {
	VAL float64
	VAH float64
}

func resolveSessionSchedule(intervals []moex.SessionInterval) (sessionSchedule, error) {
	schedule := sessionSchedule{}
	for _, interval := range intervals {
		name := strings.ToLower(interval.Name)
		switch {
		case strings.Contains(name, "основ") || strings.Contains(name, "main"):
			schedule.MainStart = interval.Start
			schedule.MainEnd = interval.End
		case strings.Contains(name, "аукцион отк") || strings.Contains(name, "open auction"):
			start := interval.Start
			end := interval.End
			schedule.AuctionOpenStart = &start
			schedule.AuctionOpenEnd = &end
		case strings.Contains(name, "аукцион зак") || strings.Contains(name, "close auction"):
			start := interval.Start
			end := interval.End
			schedule.AuctionCloseStart = &start
			schedule.AuctionCloseEnd = &end
		}
	}
	if schedule.MainStart.IsZero() || schedule.MainEnd.IsZero() {
		return schedule, errors.New("main session interval not found")
	}
	return schedule, nil
}

func (s sessionSchedule) contains(t time.Time) bool {
	if !t.Before(s.MainEnd) || t.Before(s.MainStart) {
		return false
	}
	if s.AuctionOpenStart != nil && s.AuctionOpenEnd != nil && !t.Before(*s.AuctionOpenStart) && t.Before(*s.AuctionOpenEnd) {
		return false
	}
	if s.AuctionCloseStart != nil && s.AuctionCloseEnd != nil && !t.Before(*s.AuctionCloseStart) && t.Before(*s.AuctionCloseEnd) {
		return false
	}
	return true
}

func buildMinuteSeries(candles []moex.MinuteCandle, schedule sessionSchedule, fallbacks priceFallbacks, lotSize float64) (mainSessionSeries, error) {
	if schedule.MainEnd.Before(schedule.MainStart) {
		return mainSessionSeries{}, errors.New("invalid main session bounds")
	}
	duration := schedule.MainEnd.Sub(schedule.MainStart)
	if duration <= 0 {
		return mainSessionSeries{}, errors.New("empty main session interval")
	}

	candleByBegin := make(map[time.Time]moex.MinuteCandle, len(candles))
	for _, candle := range candles {
		candleByBegin[candle.Begin] = candle
	}

	var series []minuteBar
	var lastPrice float64

	seedPrice := determineSeedPrice(schedule, candleByBegin, fallbacks)
	if seedPrice <= 0 {
		return mainSessionSeries{}, errors.New("unable to determine initial price")
	}
	lastPrice = seedPrice

	for current := schedule.MainStart; current.Before(schedule.MainEnd); current = current.Add(time.Minute) {
		if !schedule.contains(current) {
			continue
		}
		candle, ok := candleByBegin[current]
		bar := minuteBar{Time: current}
		if ok {
			bar.Open = candle.Open
			bar.High = candle.High
			bar.Low = candle.Low
			bar.Close = candle.Close
			bar.Volume = candle.Volume
			bar.Value = candle.Value

			priceProxy := priceProxy(candle)
			if bar.Value == 0 {
				bar.Value = bar.Volume * lotSize * priceProxy
			}

			if bar.Close <= 0 {
				if candle.Close > 0 {
					bar.Close = candle.Close
				} else if priceProxy > 0 {
					bar.Close = priceProxy
				} else {
					bar.Close = lastPrice
				}
			}
			if bar.Open <= 0 {
				bar.Open = fallbackPrice(candle.Open, lastPrice)
			}
			if bar.High <= 0 {
				bar.High = fallbackPrice(candle.High, bar.Close)
			}
			if bar.Low <= 0 {
				bar.Low = fallbackPrice(candle.Low, bar.Close)
			}
			if bar.Close > 0 {
				lastPrice = bar.Close
			}
		} else {
			bar.Open = lastPrice
			bar.High = lastPrice
			bar.Low = lastPrice
			bar.Close = lastPrice
		}

		if bar.Value == 0 && bar.Volume > 0 {
			bar.Value = bar.Volume * lotSize * bar.Close
		}
		if bar.Volume > 0 {
			bar.Active = true
		}
		series = append(series, bar)
	}

	if len(series) == 0 {
		return mainSessionSeries{}, errors.New("no bars inside main session")
	}

	return mainSessionSeries{Bars: series}, nil
}

func determineSeedPrice(schedule sessionSchedule, candleByBegin map[time.Time]moex.MinuteCandle, fallbacks priceFallbacks) float64 {
	first := moex.MinuteCandle{}
	hasFirst := false
	for current := schedule.MainStart; current.Before(schedule.MainEnd); current = current.Add(time.Minute) {
		if !schedule.contains(current) {
			continue
		}
		if candle, ok := candleByBegin[current]; ok {
			first = candle
			hasFirst = true
			break
		}
	}
	if hasFirst {
		if first.Close > 0 {
			return first.Close
		}
		if first.Open > 0 {
			return first.Open
		}
		price := priceProxy(first)
		if price > 0 {
			return price
		}
	}
	if fallbacks.FirstTradePrice > 0 {
		return fallbacks.FirstTradePrice
	}
	if fallbacks.PrevSessionClose > 0 {
		return fallbacks.PrevSessionClose
	}
	return 0
}

func priceProxy(c moex.MinuteCandle) float64 {
	base := c.High + c.Low + 2*c.Close
	if base == 0 {
		return 0
	}
	return base / 4
}

func fallbackPrice(value, defaultValue float64) float64 {
	if value > 0 {
		return value
	}
	return defaultValue
}

func calculateVWAP(series mainSessionSeries) (float64, error) {
	_, volumeSum, vwap := series.DailyAggregates()
	if volumeSum <= 0 {
		return 0, errors.New("zero volume for VWAP")
	}
	return vwap, nil
}

func calculateValueArea(series mainSessionSeries, minStep float64) (valueAreaResult, error) {
	if len(series.Bars) == 0 {
		return valueAreaResult{}, errors.New("empty series for value area")
	}

	step := minStep
	if step <= 0 {
		step = 0.01
	}

	volumes := make(map[float64]float64)
	totalVolume := 0.0
	for _, bar := range series.Bars {
		price := bar.Close
		if price == 0 {
			price = (bar.High + bar.Low) / 2
		}
		if price == 0 {
			continue
		}
		bucket := math.Round(price/step) * step
		volumes[bucket] += bar.Volume
		totalVolume += bar.Volume
	}
	if totalVolume <= 0 {
		return valueAreaResult{}, errors.New("no volume for value area")
	}

	type bucketVolume struct {
		Price  float64
		Volume float64
	}

	buckets := make([]bucketVolume, 0, len(volumes))
	for price, volume := range volumes {
		buckets = append(buckets, bucketVolume{Price: price, Volume: volume})
	}
	if len(buckets) == 0 {
		return valueAreaResult{}, errors.New("no buckets for value area")
	}

	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Volume == buckets[j].Volume {
			return buckets[i].Price < buckets[j].Price
		}
		return buckets[i].Volume > buckets[j].Volume
	})

	maxBucket := buckets[0]
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Price < buckets[j].Price
	})

	centerIndex := 0
	for i, bucket := range buckets {
		if bucket.Price == maxBucket.Price {
			centerIndex = i
			break
		}
	}

	target := totalVolume * valueAreaCoverageRatio
	accumulated := buckets[centerIndex].Volume
	lower := centerIndex
	upper := centerIndex

	for accumulated < target {
		var nextLower float64 = -1
		var nextUpper float64 = -1
		if lower > 0 {
			nextLower = buckets[lower-1].Volume
		}
		if upper < len(buckets)-1 {
			nextUpper = buckets[upper+1].Volume
		}
		if nextLower < 0 && nextUpper < 0 {
			break
		}
		if nextUpper >= nextLower {
			upper++
			if upper >= len(buckets) {
				upper = len(buckets) - 1
				break
			}
			accumulated += buckets[upper].Volume
		} else {
			lower--
			if lower < 0 {
				lower = 0
				break
			}
			accumulated += buckets[lower].Volume
		}
	}

	return valueAreaResult{VAL: buckets[lower].Price, VAH: buckets[upper].Price}, nil
}

func firstTradePrice(trades []moex.Trade, schedule sessionSchedule) float64 {
	if len(trades) == 0 {
		return 0
	}
	sortedTrades := make([]moex.Trade, len(trades))
	copy(sortedTrades, trades)
	sort.Slice(sortedTrades, func(i, j int) bool {
		return sortedTrades[i].Time.Before(sortedTrades[j].Time)
	})
	for _, trade := range sortedTrades {
		if trade.Price <= 0 {
			continue
		}
		if schedule.contains(trade.Time) {
			return trade.Price
		}
	}
	return 0
}

func formatFloat(value float64) *string {
	if math.IsNaN(value) {
		return nil
	}
	formatted := fmt.Sprintf("%.6f", value)
	return &formatted
}
