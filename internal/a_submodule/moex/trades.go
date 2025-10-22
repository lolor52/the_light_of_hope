package moex

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

type Trade struct {
	Time     time.Time
	Price    float64
	Quantity float64
}

// GetTrades возвращает сделки за указанную дату.
func (c *Client) GetTrades(ctx context.Context, boardID, secID string, date time.Time) ([]Trade, error) {
	endpoint := fmt.Sprintf("engines/stock/markets/shares/boards/%s/securities/%s/trades.json", boardID, secID)
	values := url.Values{}
	values.Set("date", date.Format("2006-01-02"))

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return nil, fmt.Errorf("load moscow location: %w", err)
	}

	var trades []Trade
	start := 0
	for {
		if start > 0 {
			values.Set("start", strconv.Itoa(start))
		} else {
			values.Del("start")
		}

		var response struct {
			Trades struct {
				Columns []string        `json:"columns"`
				Data    [][]interface{} `json:"data"`
			} `json:"trades"`
		}

		if err := c.getJSON(ctx, endpoint, values, &response); err != nil {
			return nil, err
		}

		if len(response.Trades.Data) == 0 {
			break
		}

		idx := columnIndexInsensitive(response.Trades.Columns)
		dateIdx, okDate := idx["TRADEDATE"]
		timeIdx, okTime := idx["TIME"]
		if !okTime {
			timeIdx, okTime = idx["TRADETIME"]
		}
		if !okTime {
			timeIdx, okTime = idx["SYSTIME"]
		}
		if !okTime {
			timeIdx = -1
		}
		priceIdx, okPrice := idx["PRICE"]
		if !okPrice {
			priceIdx = -1
		}
		qtyIdx, okQty := idx["QUANTITY"]
		if !okQty {
			qtyIdx = -1
		}

		for _, row := range response.Trades.Data {
			var timestamp time.Time
			if okDate && timeIdx >= 0 {
				tradeDate := stringFromRow(row, dateIdx)
				tradeTime := stringFromRow(row, timeIdx)
				if tradeTime == "" {
					if altIdx, ok := idx["SYSTIME"]; ok {
						tradeTime = stringFromRow(row, altIdx)
					}
				}
				if tradeDate != "" && tradeTime != "" {
					combined := fmt.Sprintf("%s %s", tradeDate, tradeTime)
					parsed, err := time.ParseInLocation("2006-01-02 15:04:05", combined, loc)
					if err == nil {
						timestamp = parsed
					}
				}
			}
			if timestamp.IsZero() {
				continue
			}
			trade := Trade{Time: timestamp}
			if priceIdx >= 0 {
				trade.Price = floatFromRow(row, priceIdx)
			}
			if qtyIdx >= 0 {
				trade.Quantity = floatFromRow(row, qtyIdx)
			}
			trades = append(trades, trade)
		}

		start += len(response.Trades.Data)
	}

	return trades, nil
}
