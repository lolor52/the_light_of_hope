package alor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	apiBaseURL      = "https://api.alor.ru"
	requestTimeout  = 30 * time.Second
	defaultPageSize = 5000
)

const (
	// ExchangeMOEX обозначает Московскую биржу.
	ExchangeMOEX = "MOEX"
	// ExchangeSPB обозначает Санкт-Петербургскую биржу.
	ExchangeSPB = "SPB"
)

// Client инкапсулирует обращение к ALOR OpenAPI для получения рыночных данных.
type Client struct {
	httpClient *http.Client
	token      string
}

// Instrument описывает инструмент на бирже ALOR.
type Instrument struct {
	Exchange string
	Board    string
	Symbol   string
}

// Trade описывает сделку, полученную из ALOR OpenAPI.
type Trade struct {
	Price          float64
	Quantity       float64
	Value          float64
	TradingSession string
	TradeTime      string
}

// NewClient создаёт клиента ALOR OpenAPI с заданным токеном доступа.
func NewClient(token string) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("alor: empty token")
	}

	client := &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		token:      token,
	}

	return client, nil
}

// Trades возвращает список сделок инструмента за выбранную торговую дату.
func (c *Client) Trades(ctx context.Context, instrument Instrument, sessionDate time.Time) ([]Trade, error) {
	if c == nil {
		return nil, fmt.Errorf("alor: client is not configured")
	}
	if err := instrument.validate(); err != nil {
		return nil, err
	}

	loc := moscowLocation()
	if loc == nil {
		return nil, fmt.Errorf("alor: moscow location unavailable")
	}

	sessionDay := sessionDate.In(loc)
	dayStart := time.Date(sessionDay.Year(), sessionDay.Month(), sessionDay.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	var result []Trade
	cursor := dayStart

	for {
		chunk, lastMoment, err := c.fetchTrades(ctx, instrument, cursor, dayEnd)
		if err != nil {
			return nil, err
		}
		if len(chunk) == 0 {
			break
		}

		result = append(result, chunk...)

		if !lastMoment.After(cursor) {
			break
		}

		cursor = lastMoment.Add(time.Millisecond)
		if !cursor.Before(dayEnd) {
			break
		}
	}

	return result, nil
}

func (c *Client) fetchTrades(ctx context.Context, instrument Instrument, from, to time.Time) ([]Trade, time.Time, error) {
	endpoint := buildEndpoint(instrument)

	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339Nano))
	query.Set("to", to.UTC().Format(time.RFC3339Nano))
	query.Set("limit", fmt.Sprintf("%d", defaultPageSize))
	query.Set("direction", "1")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("alor: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	log.Printf("alor: запрос %s %s?%s", http.MethodGet, endpoint, query.Encode())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("alor: send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("alor: read response: %w", err)
	}

	log.Printf("alor: ответ %s статус=%s тело=%s", endpoint, resp.Status, string(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("alor: trades status %s", resp.Status)
	}

	var payload []rawTrade
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return nil, time.Time{}, fmt.Errorf("alor: decode trades: %w", err)
	}

	trades := make([]Trade, 0, len(payload))
	var lastMoment time.Time

	for i, item := range payload {
		tradeMoment, err := item.parseMoment()
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("alor: trade moment: %w", err)
		}
		if i == 0 || tradeMoment.After(lastMoment) {
			lastMoment = tradeMoment
		}

		loc := moscowLocation()
		if loc == nil {
			return nil, time.Time{}, fmt.Errorf("alor: moscow location unavailable")
		}

		tradeTime := tradeMoment.In(loc).Format("15:04:05")

		trades = append(trades, Trade{
			Price:          item.Price,
			Quantity:       item.quantity(),
			Value:          item.tradeValue(),
			TradingSession: item.session(),
			TradeTime:      tradeTime,
		})
	}

	return trades, lastMoment, nil
}

func buildEndpoint(instrument Instrument) string {
	var builder strings.Builder
	builder.Grow(len(apiBaseURL) + 32)
	builder.WriteString(strings.TrimRight(apiBaseURL, "/"))
	builder.WriteString("/md/v2/Securities/")
	builder.WriteString(url.PathEscape(strings.ToUpper(instrument.Exchange)))
	builder.WriteByte('/')
	builder.WriteString(url.PathEscape(strings.ToUpper(instrument.Board)))
	builder.WriteByte('/')
	builder.WriteString(url.PathEscape(strings.ToUpper(instrument.Symbol)))
	builder.WriteString("/alltrades")
	return builder.String()
}

func (i Instrument) validate() error {
	if strings.TrimSpace(i.Exchange) == "" {
		return fmt.Errorf("alor: empty instrument exchange")
	}
	if strings.TrimSpace(i.Board) == "" {
		return fmt.Errorf("alor: empty instrument board")
	}
	if strings.TrimSpace(i.Symbol) == "" {
		return fmt.Errorf("alor: empty instrument symbol")
	}
	return nil
}

type rawTrade struct {
	Price          float64 `json:"price"`
	Quantity       float64 `json:"qty"`
	QuantityAlt    float64 `json:"quantity"`
	Value          float64 `json:"value"`
	ValueAlt       float64 `json:"val"`
	Moment         string  `json:"moment"`
	TradeTime      int64   `json:"tradeTime"`
	Session        string  `json:"session"`
	TradingSession string  `json:"tradingSession"`
}

func (t rawTrade) session() string {
	if t.TradingSession != "" {
		return t.TradingSession
	}
	return t.Session
}

func (t rawTrade) quantity() float64 {
	if t.Quantity != 0 {
		return t.Quantity
	}
	return t.QuantityAlt
}

func (t rawTrade) tradeValue() float64 {
	if t.Value != 0 {
		return t.Value
	}
	if t.ValueAlt != 0 {
		return t.ValueAlt
	}
	return t.Price * t.quantity()
}

func (t rawTrade) parseMoment() (time.Time, error) {
	if t.Moment != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, t.Moment); err == nil {
			return parsed, nil
		}
		if parsed, err := time.Parse(time.RFC3339, t.Moment); err == nil {
			return parsed, nil
		}
	}
	if t.TradeTime > 0 {
		return time.Unix(0, t.TradeTime*int64(time.Millisecond)), nil
	}
	return time.Time{}, fmt.Errorf("moment is empty")
}

var (
	moscowOnce sync.Once
	moscowLoc  *time.Location
)

func moscowLocation() *time.Location {
	moscowOnce.Do(func() {
		loc, err := time.LoadLocation("Europe/Moscow")
		if err != nil {
			moscowLoc = time.FixedZone("MSK", 3*60*60)
			return
		}
		moscowLoc = loc
	})
	return moscowLoc
}
