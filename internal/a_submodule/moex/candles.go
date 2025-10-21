package moex

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"
)

// MinuteCandle описывает минутную свечу.
type MinuteCandle struct {
	Begin  time.Time
	End    time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Value  float64
	Volume float64
}

// DailyCandle описывает дневную свечу из endpoints candles.
type DailyCandle struct {
	Begin  time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// GetMinuteCandles возвращает список минутных свечей для конкретного дня.
func (c *Client) GetMinuteCandles(ctx context.Context, boardID, secID string, date time.Time) ([]MinuteCandle, error) {
	endpoint := fmt.Sprintf("engines/stock/markets/shares/boards/%s/securities/%s/candles.json", boardID, secID)
	values := url.Values{}
	values.Set("from", date.Format("2006-01-02"))
	values.Set("till", date.Format("2006-01-02"))
	values.Set("interval", "1")

	var response struct {
		Candles struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"candles"`
	}

	if err := c.getJSON(ctx, endpoint, values, &response); err != nil {
		return nil, err
	}

	idx := columnIndex(response.Candles.Columns)
	candles := make([]MinuteCandle, 0, len(response.Candles.Data))
	for _, row := range response.Candles.Data {
		begin, err := parseTime(row[idx["begin"]])
		if err != nil {
			return nil, fmt.Errorf("parse candle begin: %w", err)
		}
		end, err := parseTime(row[idx["end"]])
		if err != nil {
			return nil, fmt.Errorf("parse candle end: %w", err)
		}
		candles = append(candles, MinuteCandle{
			Begin:  begin,
			End:    end,
			Open:   floatFromRow(row, idx["open"]),
			High:   floatFromRow(row, idx["high"]),
			Low:    floatFromRow(row, idx["low"]),
			Close:  floatFromRow(row, idx["close"]),
			Value:  floatFromRow(row, idx["value"]),
			Volume: floatFromRow(row, idx["volume"]),
		})
	}

	sort.Slice(candles, func(i, j int) bool {
		return candles[i].Begin.Before(candles[j].Begin)
	})

	return candles, nil
}

// GetDailyCandles возвращает дневные свечи за период.
func (c *Client) GetDailyCandles(ctx context.Context, boardID, secID string, from, till time.Time) ([]DailyCandle, error) {
	endpoint := fmt.Sprintf("engines/stock/markets/shares/boards/%s/securities/%s/candles.json", boardID, secID)
	values := url.Values{}
	values.Set("from", from.Format("2006-01-02"))
	values.Set("till", till.Format("2006-01-02"))
	values.Set("interval", "24")

	var response struct {
		Candles struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"candles"`
	}

	if err := c.getJSON(ctx, endpoint, values, &response); err != nil {
		return nil, err
	}

	idx := columnIndex(response.Candles.Columns)
	candles := make([]DailyCandle, 0, len(response.Candles.Data))
	for _, row := range response.Candles.Data {
		begin, err := parseTime(row[idx["begin"]])
		if err != nil {
			return nil, fmt.Errorf("parse daily candle begin: %w", err)
		}
		candles = append(candles, DailyCandle{
			Begin:  begin,
			Open:   floatFromRow(row, idx["open"]),
			High:   floatFromRow(row, idx["high"]),
			Low:    floatFromRow(row, idx["low"]),
			Close:  floatFromRow(row, idx["close"]),
			Volume: floatFromRow(row, idx["volume"]),
		})
	}

	sort.Slice(candles, func(i, j int) bool {
		return candles[i].Begin.Before(candles[j].Begin)
	})

	return candles, nil
}

func parseTime(value interface{}) (time.Time, error) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return time.Time{}, fmt.Errorf("empty time value")
		}
		t, err := time.Parse("2006-01-02 15:04:05", v)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse time: %w", err)
		}
		return t, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported time value %T", value)
	}
}
