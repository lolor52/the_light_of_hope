package moex

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// MarketData содержит данные книги заявок верхнего уровня.
type MarketData struct {
	Bid   float64
	Offer float64
	Last  float64
}

// OrderBookEntry описывает одну ступень стакана.
type OrderBookEntry struct {
	Price    float64
	Quantity float64
}

// OrderBook описывает верхние уровни стакана.
type OrderBook struct {
	Bids []OrderBookEntry
	Asks []OrderBookEntry
}

// SecurityInfo хранит параметры инструмента.
type SecurityInfo struct {
	LotSize float64
	MinStep float64
}

// GetSecurityInfo загружает базовые параметры инструмента.
func (c *Client) GetSecurityInfo(ctx context.Context, boardID, secID string) (SecurityInfo, error) {
	endpoint := fmt.Sprintf("engines/stock/markets/shares/boards/%s/securities/%s.json", boardID, secID)
	values := url.Values{}
	values.Set("iss.only", "securities")

	var response struct {
		Securities struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"securities"`
	}

	if err := c.getJSON(ctx, endpoint, values, &response); err != nil {
		return SecurityInfo{}, err
	}

	info := SecurityInfo{}
	if len(response.Securities.Data) == 0 {
		return info, fmt.Errorf("security info empty")
	}

	idx := columnIndex(response.Securities.Columns)
	row := response.Securities.Data[0]

	info.LotSize = floatFromRow(row, idx["LOTSIZE"])
	info.MinStep = floatFromRow(row, idx["MINSTEP"])

	return info, nil
}

// GetMarketData возвращает BID/OFFER на указанную дату.
func (c *Client) GetMarketData(ctx context.Context, boardID, secID string, date time.Time) (MarketData, error) {
	endpoint := fmt.Sprintf("engines/stock/markets/shares/boards/%s/securities/%s.json", boardID, secID)
	values := url.Values{}
	values.Set("iss.only", "marketdata")
	values.Set("date", date.Format("2006-01-02"))

	var response struct {
		MarketData struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"marketdata"`
	}

	if err := c.getJSON(ctx, endpoint, values, &response); err != nil {
		return MarketData{}, err
	}

	if len(response.MarketData.Data) == 0 {
		return MarketData{}, fmt.Errorf("marketdata empty")
	}

	idx := columnIndex(response.MarketData.Columns)
	row := response.MarketData.Data[0]

	return MarketData{
		Bid:   floatFromRow(row, idx["BID"]),
		Offer: floatFromRow(row, idx["OFFER"]),
		Last:  floatFromRow(row, idx["LAST"]),
	}, nil
}

// GetOrderBook возвращает уровни стакана на указанную дату.
func (c *Client) GetOrderBook(ctx context.Context, boardID, secID string, date time.Time, depth int) (OrderBook, error) {
	endpoint := fmt.Sprintf("engines/stock/markets/shares/boards/%s/securities/%s.json", boardID, secID)
	values := url.Values{}
	values.Set("iss.only", "orderbook")
	values.Set("date", date.Format("2006-01-02"))
	if depth > 0 {
		values.Set("depth", fmt.Sprintf("%d", depth))
	}

	var response struct {
		OrderBook struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"orderbook"`
	}

	if err := c.getJSON(ctx, endpoint, values, &response); err != nil {
		return OrderBook{}, err
	}

	book := OrderBook{}
	idx := columnIndex(response.OrderBook.Columns)
	for _, row := range response.OrderBook.Data {
		side := stringFromRow(row, idx["TYPE"])
		entry := OrderBookEntry{
			Price:    floatFromRow(row, idx["PRICE"]),
			Quantity: floatFromRow(row, idx["QUANTITY"]),
		}
		switch strings.ToUpper(side) {
		case "BID":
			book.Bids = append(book.Bids, entry)
		case "OFFER":
			book.Asks = append(book.Asks, entry)
		}
	}

	return book, nil
}
