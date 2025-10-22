package indicators

import (
	"math"
	"time"
)

type MinuteBar struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
	Value  float64
	Active bool
}

type SessionSeries struct {
	Bars []MinuteBar
}

func (series SessionSeries) Length() int {
	return len(series.Bars)
}

func (series SessionSeries) TotalValue() float64 {
	total := 0.0
	for _, bar := range series.Bars {
		total += bar.Value
	}
	return total
}

func (series SessionSeries) ActiveCount() int {
	count := 0
	for _, bar := range series.Bars {
		if bar.Active {
			count++
		}
	}
	return count
}

func (series SessionSeries) ActiveValues(fn func(MinuteBar) (float64, bool)) []float64 {
	result := make([]float64, 0, len(series.Bars))
	for _, bar := range series.Bars {
		if !bar.Active {
			continue
		}
		if value, ok := fn(bar); ok {
			result = append(result, value)
		}
	}
	return result
}

func (series SessionSeries) SessionExtremes() (float64, float64) {
	high := math.Inf(-1)
	low := math.Inf(1)
	for _, bar := range series.Bars {
		if bar.High > high {
			high = bar.High
		}
		if bar.Low < low {
			low = bar.Low
		}
	}
	return high, low
}

func (series SessionSeries) CumulativeValues() ([]float64, []float64) {
	values := make([]float64, len(series.Bars))
	volumes := make([]float64, len(series.Bars))
	for i, bar := range series.Bars {
		values[i] = bar.Value
		volumes[i] = bar.Volume
		if i > 0 {
			values[i] += values[i-1]
			volumes[i] += volumes[i-1]
		}
	}
	return values, volumes
}

func (series SessionSeries) Closes() []float64 {
	closes := make([]float64, len(series.Bars))
	for i, bar := range series.Bars {
		closes[i] = bar.Close
	}
	return closes
}

func (series SessionSeries) Values() []float64 {
	vals := make([]float64, len(series.Bars))
	for i, bar := range series.Bars {
		vals[i] = bar.Value
	}
	return vals
}

func (series SessionSeries) Volumes() []float64 {
	volumes := make([]float64, len(series.Bars))
	for i, bar := range series.Bars {
		volumes[i] = bar.Volume
	}
	return volumes
}

func (series SessionSeries) LogReturns() []float64 {
	results := make([]float64, len(series.Bars))
	prevClose := math.NaN()
	for i, bar := range series.Bars {
		if i == 0 {
			results[i] = math.NaN()
			prevClose = bar.Close
			continue
		}
		if bar.Close <= 0 || prevClose <= 0 {
			results[i] = math.NaN()
		} else {
			results[i] = math.Abs(math.Log(bar.Close / prevClose))
		}
		prevClose = bar.Close
	}
	return results
}

func (series SessionSeries) PriceRef() float64 {
	closes := make([]float64, 0, len(series.Bars))
	for _, bar := range series.Bars {
		if bar.Active {
			closes = append(closes, bar.Close)
		}
	}
	return Median(closes)
}

func (series SessionSeries) DailyAggregates() (valueSum, volumeSum, vwap float64) {
	for _, bar := range series.Bars {
		valueSum += bar.Value
		volumeSum += bar.Volume
	}
	if volumeSum > 0 {
		vwap = valueSum / volumeSum
	}
	return
}
