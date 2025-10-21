package moex

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// HistoryRow содержит агрегированные дневные данные по тикеру.
type HistoryRow struct {
	BoardID   string
	TradeDate time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Value     float64
	VWAP      float64
}

// GetHistoryRow загружает данные истории для конкретной даты.
func (c *Client) GetHistoryRow(ctx context.Context, boardID, secID string, date time.Time) (*HistoryRow, error) {
	endpoint := fmt.Sprintf("history/engines/stock/markets/shares/boards/%s/securities/%s.json", boardID, secID)
	values := url.Values{}
	day := date.Format("2006-01-02")
	values.Set("from", day)
	values.Set("till", day)

	var response struct {
		History struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"history"`
	}

	if err := c.getJSON(ctx, endpoint, values, &response); err != nil {
		return nil, err
	}

	if len(response.History.Data) == 0 {
		return nil, nil
	}

	idx := columnIndex(response.History.Columns)
	row := response.History.Data[0]

	tradeDate, err := parseDate(row[idx["TRADEDATE"]])
	if err != nil {
		return nil, fmt.Errorf("parse tradedate: %w", err)
	}

	result := &HistoryRow{
		BoardID:   boardID,
		TradeDate: tradeDate,
		Open:      floatFromRow(row, idx["OPEN"]),
		High:      floatFromRow(row, idx["HIGH"]),
		Low:       floatFromRow(row, idx["LOW"]),
		Close:     floatFromRow(row, idx["CLOSE"]),
		Volume:    floatFromRow(row, idx["VOLUME"]),
		Value:     floatFromRow(row, idx["VALUE"]),
		VWAP:      floatFromRow(row, idx["WAPRICE"]),
	}

	return result, nil
}

// GetHistoryWindow возвращает последовательность дневных баров за указанный период.
func (c *Client) GetHistoryWindow(ctx context.Context, boardID, secID string, from, till time.Time) ([]HistoryRow, error) {
	endpoint := fmt.Sprintf("history/engines/stock/markets/shares/boards/%s/securities/%s.json", boardID, secID)
	values := url.Values{}
	values.Set("from", from.Format("2006-01-02"))
	values.Set("till", till.Format("2006-01-02"))

	var response struct {
		History struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"history"`
	}

	if err := c.getJSON(ctx, endpoint, values, &response); err != nil {
		return nil, err
	}

	idx := columnIndex(response.History.Columns)
	rows := make([]HistoryRow, 0, len(response.History.Data))
	for _, dataRow := range response.History.Data {
		tradeDate, err := parseDate(dataRow[idx["TRADEDATE"]])
		if err != nil {
			return nil, fmt.Errorf("parse tradedate: %w", err)
		}
		rows = append(rows, HistoryRow{
			BoardID:   boardID,
			TradeDate: tradeDate,
			Open:      floatFromRow(dataRow, idx["OPEN"]),
			High:      floatFromRow(dataRow, idx["HIGH"]),
			Low:       floatFromRow(dataRow, idx["LOW"]),
			Close:     floatFromRow(dataRow, idx["CLOSE"]),
			Volume:    floatFromRow(dataRow, idx["VOLUME"]),
			Value:     floatFromRow(dataRow, idx["VALUE"]),
			VWAP:      floatFromRow(dataRow, idx["WAPRICE"]),
		})
	}

	return rows, nil
}
