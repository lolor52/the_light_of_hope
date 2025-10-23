package indicators

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"invest_intraday/internal/a_submodule/alor"
)

// SwingCountParams задаёт параметры адаптивного ZigZag.
type SwingCountParams struct {
	ATRWindow           int
	ThresholdMultiplier float64
}

// DefaultSwingCountParams возвращает рекомендуемые значения параметров.
func DefaultSwingCountParams() SwingCountParams {
	return SwingCountParams{
		ATRWindow:           14,
		ThresholdMultiplier: 1.0,
	}
}

// SwingCountPairedCalculator рассчитывает количество валидных пар свингов за сессию.
type SwingCountPairedCalculator struct {
	fetcher *sessionFetcher
	params  SwingCountParams
}

// NewSwingCountPairedCalculator создаёт расчётчик Swing Count Paired.
func NewSwingCountPairedCalculator(tickerRepo tickerInfoProvider, marketClient tradeProvider, params SwingCountParams) *SwingCountPairedCalculator {
	normalized := normalizeSwingParams(params)
	fetcher := newSessionFetcher(tickerRepo, marketClient)
	return &SwingCountPairedCalculator{
		fetcher: fetcher,
		params:  normalized,
	}
}

// Calculate возвращает SwingCountPairs для основной торговой сессии.
func (c *SwingCountPairedCalculator) Calculate(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (int, error) {
	if c == nil || c.fetcher == nil {
		return 0, fmt.Errorf("swing calculator is not fully configured")
	}

	_, trades, err := c.fetcher.mainSessionTrades(ctx, tickerInfoID, sessionDate)
	if err != nil {
		return 0, err
	}

	minuteBars, err := buildMinuteBars(trades, sessionDate)
	if err != nil {
		return 0, err
	}

	return swingCountPairs(minuteBars, c.params), nil
}

func normalizeSwingParams(params SwingCountParams) SwingCountParams {
	normalized := params
	defaults := DefaultSwingCountParams()
	if normalized.ATRWindow <= 0 {
		normalized.ATRWindow = defaults.ATRWindow
	}
	if normalized.ThresholdMultiplier <= 0 {
		normalized.ThresholdMultiplier = defaults.ThresholdMultiplier
	}
	return normalized
}

type minuteBar struct {
	timestamp  time.Time
	high       float64
	low        float64
	close      float64
	volume     float64
	tradeCount int
}

func buildMinuteBars(trades []alor.Trade, sessionDate time.Time) ([]minuteBar, error) {
	if len(trades) == 0 {
		return nil, ErrNoTrades
	}

	type timedTrade struct {
		trade alor.Trade
		ts    time.Time
	}

	var timed []timedTrade
	for _, trade := range trades {
		if trade.Quantity <= 0 {
			continue
		}

		parsed, err := parseMoscowTime(trade.TradeTime)
		if err != nil {
			return nil, fmt.Errorf("parse trade time: %w", err)
		}

		sessionDay := sessionDate.In(parsed.Location())
		ts := time.Date(
			sessionDay.Year(),
			sessionDay.Month(),
			sessionDay.Day(),
			parsed.Hour(),
			parsed.Minute(),
			parsed.Second(),
			0,
			parsed.Location(),
		)

		timed = append(timed, timedTrade{trade: trade, ts: ts})
	}

	if len(timed) == 0 {
		return nil, ErrNoTrades
	}

	sort.SliceStable(timed, func(i, j int) bool {
		return timed[i].ts.Before(timed[j].ts)
	})

	barsMap := make(map[time.Time]*minuteBar)
	order := make([]time.Time, 0)

	for _, item := range timed {
		minute := item.ts.Truncate(time.Minute)
		bar, ok := barsMap[minute]
		if !ok {
			bar = &minuteBar{timestamp: minute}
			barsMap[minute] = bar
			order = append(order, minute)
		}

		price := item.trade.Price
		if bar.tradeCount == 0 {
			bar.high = price
			bar.low = price
			bar.close = price
		} else {
			if price > bar.high {
				bar.high = price
			}
			if price < bar.low {
				bar.low = price
			}
			bar.close = price
		}
		bar.volume += item.trade.Quantity
		bar.tradeCount++
	}

	sort.Slice(order, func(i, j int) bool {
		return order[i].Before(order[j])
	})

	bars := make([]minuteBar, 0, len(order))
	for _, key := range order {
		bar := barsMap[key]
		if bar.tradeCount == 0 {
			continue
		}
		bars = append(bars, *bar)
	}

	if len(bars) == 0 {
		return nil, ErrNoTrades
	}

	return bars, nil
}

type swingBar struct {
	high  float64
	low   float64
	close float64
	vwap  float64
	theta float64
	atr   float64
}

func swingCountPairs(bars []minuteBar, params SwingCountParams) int {
	if len(bars) < 2 {
		return 0
	}

	calcBars := make([]swingBar, len(bars))
	var sumTPV float64
	var sumVolume float64
	var prevClose float64
	var atr float64
	var trSum float64

	for i, bar := range bars {
		tp := (bar.high + bar.low + bar.close) / 3

		if bar.volume > 0 {
			sumTPV += tp * bar.volume
			sumVolume += bar.volume
			if sumVolume > 0 {
				calcBars[i].vwap = sumTPV / sumVolume
			} else {
				calcBars[i].vwap = tp
			}
		} else {
			if i == 0 {
				calcBars[i].vwap = tp
			} else {
				calcBars[i].vwap = calcBars[i-1].vwap
			}
		}

		var tr float64
		if i == 0 {
			tr = bar.high - bar.low
		} else {
			tr = math.Max(bar.high-bar.low, math.Max(math.Abs(bar.high-prevClose), math.Abs(bar.low-prevClose)))
		}

		trSum += tr

		if len(bars) < params.ATRWindow {
			atr = trSum / float64(i+1)
		} else if i+1 < params.ATRWindow {
			atr = trSum / float64(i+1)
		} else if i+1 == params.ATRWindow {
			atr = trSum / float64(params.ATRWindow)
		} else {
			atr = ((atr * float64(params.ATRWindow-1)) + tr) / float64(params.ATRWindow)
		}

		calcBars[i].high = bar.high
		calcBars[i].low = bar.low
		calcBars[i].close = bar.close
		calcBars[i].atr = atr
		calcBars[i].theta = params.ThresholdMultiplier * atr
		prevClose = bar.close
	}

	pivots := buildPivots(calcBars)
	validSwings := countValidSwings(calcBars, pivots)
	if validSwings < 2 {
		return 0
	}
	return validSwings / 2
}

type pivot struct {
	index  int
	value  float64
	isHigh bool
}

func buildPivots(bars []swingBar) []pivot {
	if len(bars) < 2 {
		return nil
	}

	c1 := bars[0].close
	t0 := -1
	for i := 1; i < len(bars); i++ {
		if math.Abs(bars[i].close-c1) >= bars[i].theta {
			t0 = i
			break
		}
	}
	if t0 == -1 {
		return nil
	}

	var pivots []pivot
	const (
		dirUp   = 1
		dirDown = -1
	)

	var direction int
	var currentExtreme float64
	var currentIndex int

	if bars[t0].close > c1 {
		direction = dirUp
		minLow := bars[0].low
		minIndex := 0
		for i := 0; i <= t0; i++ {
			if bars[i].low < minLow || (bars[i].low == minLow && i > minIndex) {
				minLow = bars[i].low
				minIndex = i
			}
		}
		pivots = append(pivots, pivot{index: minIndex, value: minLow, isHigh: false})
		currentExtreme = bars[t0].high
		currentIndex = t0
	} else {
		direction = dirDown
		maxHigh := bars[0].high
		maxIndex := 0
		for i := 0; i <= t0; i++ {
			if bars[i].high > maxHigh || (bars[i].high == maxHigh && i > maxIndex) {
				maxHigh = bars[i].high
				maxIndex = i
			}
		}
		pivots = append(pivots, pivot{index: maxIndex, value: maxHigh, isHigh: true})
		currentExtreme = bars[t0].low
		currentIndex = t0
	}

	for t := t0 + 1; t < len(bars); t++ {
		bar := bars[t]
		switch direction {
		case dirUp:
			if bar.high > currentExtreme || (bar.high == currentExtreme && t > currentIndex) {
				currentExtreme = bar.high
				currentIndex = t
			}
			if currentExtreme-bar.close >= bar.theta {
				if len(pivots) == 0 || pivots[len(pivots)-1].index < currentIndex {
					pivots = append(pivots, pivot{index: currentIndex, value: currentExtreme, isHigh: true})
				}
				direction = dirDown
				currentExtreme = bar.low
				currentIndex = t
			}
		case dirDown:
			if bar.low < currentExtreme || (bar.low == currentExtreme && t > currentIndex) {
				currentExtreme = bar.low
				currentIndex = t
			}
			if bar.close-currentExtreme >= bar.theta {
				if len(pivots) == 0 || pivots[len(pivots)-1].index < currentIndex {
					pivots = append(pivots, pivot{index: currentIndex, value: currentExtreme, isHigh: false})
				}
				direction = dirUp
				currentExtreme = bar.high
				currentIndex = t
			}
		}
	}

	return pivots
}

func countValidSwings(bars []swingBar, pivots []pivot) int {
	if len(pivots) < 2 {
		return 0
	}

	swings := 0
	for i := 1; i < len(pivots); i++ {
		start := pivots[i-1].index
		end := pivots[i].index
		if end <= start {
			continue
		}
		for t := start + 1; t <= end; t++ {
			prevDiff := bars[t-1].close - bars[t-1].vwap
			currDiff := bars[t].close - bars[t].vwap
			if prevDiff*currDiff <= 0 {
				if !(prevDiff == 0 && currDiff == 0) {
					swings++
					break
				}
			}
		}
	}

	return swings
}
