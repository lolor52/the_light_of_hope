package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const issBaseURL = "https://iss.moex.com/iss"

// Trade описывает сделку, возвращаемую MOEX ISS.
type Trade struct {
	Time     time.Time
	Price    float64
	Quantity float64
}

type dataset struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
}

type tradesResponse struct {
	Trades dataset `json:"trades"`
	Cursor dataset `json:"trades.cursor"`
}

// ISSClient выполняет запросы к MOEX ISS с авторизацией Passport.
type ISSClient struct {
	httpClient *http.Client
	token      Token
}

// NewISSClient создаёт клиент ISS на основе токена Passport.
func NewISSClient(httpClient *http.Client, token Token) *ISSClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &ISSClient{
		httpClient: httpClient,
		token:      token,
	}
}

// Trades возвращает список сделок за указанную дату и доску торгов.
func (c *ISSClient) Trades(ctx context.Context, boardID, secID string, date time.Time) ([]Trade, error) {
	if strings.TrimSpace(boardID) == "" {
		return nil, fmt.Errorf("boardID is empty")
	}
	if strings.TrimSpace(secID) == "" {
		return nil, fmt.Errorf("secID is empty")
	}

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.FixedZone("MSK", 3*60*60)
	}

	day := date.In(loc)
	dateStr := day.Format("2006-01-02")

	start := 0
	const limitStep = 100
	var trades []Trade

	for {
		endpoint := fmt.Sprintf("%s/engines/stock/markets/shares/boards/%s/securities/%s/trades.json", issBaseURL, url.PathEscape(boardID), url.PathEscape(secID))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("create trades request: %w", err)
		}

		q := req.URL.Query()
		q.Set("date", dateStr)
		q.Set("start", strconv.Itoa(start))
		req.URL.RawQuery = q.Encode()

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token.AccessToken))

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("do trades request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read trades response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("trades status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var payload tradesResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decode trades response: %w", err)
		}

		pageTrades, err := parseTradesDataset(payload.Trades, day.Location(), day)
		if err != nil {
			return nil, err
		}
		trades = append(trades, pageTrades...)

		pageSize := datasetValueInt(payload.Cursor, "PAGESIZE")
		total := datasetValueInt(payload.Cursor, "TOTAL")

		if pageSize <= 0 {
			pageSize = limitStep
		}

		start += pageSize
		if start >= total || len(pageTrades) == 0 {
			break
		}
	}

	return trades, nil
}

func parseTradesDataset(ds dataset, loc *time.Location, date time.Time) ([]Trade, error) {
	idxTime := datasetColumnIndex(ds, "TRADETIME")
	idxPrice := datasetColumnIndex(ds, "PRICE")
	idxQuantity := datasetColumnIndex(ds, "QUANTITY")

	if idxTime == -1 || idxPrice == -1 || idxQuantity == -1 {
		return nil, fmt.Errorf("missing required trades columns")
	}

	var trades []Trade
	for _, row := range ds.Data {
		if len(row) <= idxQuantity {
			continue
		}

		timeStr, ok := row[idxTime].(string)
		if !ok || strings.TrimSpace(timeStr) == "" {
			continue
		}

		tradeTime, err := time.ParseInLocation("15:04:05", timeStr, loc)
		if err != nil {
			return nil, fmt.Errorf("parse trade time %q: %w", timeStr, err)
		}

		tradeTime = time.Date(date.Year(), date.Month(), date.Day(), tradeTime.Hour(), tradeTime.Minute(), tradeTime.Second(), tradeTime.Nanosecond(), loc)

		price, err := toFloat(row[idxPrice])
		if err != nil {
			return nil, fmt.Errorf("parse trade price: %w", err)
		}

		quantity, err := toFloat(row[idxQuantity])
		if err != nil {
			return nil, fmt.Errorf("parse trade quantity: %w", err)
		}

		trades = append(trades, Trade{
			Time:     tradeTime,
			Price:    price,
			Quantity: quantity,
		})
	}

	return trades, nil
}

func datasetColumnIndex(ds dataset, column string) int {
	for i, col := range ds.Columns {
		if strings.EqualFold(col, column) {
			return i
		}
	}
	return -1
}

func datasetValueInt(ds dataset, column string) int {
	idx := datasetColumnIndex(ds, column)
	if idx == -1 {
		return 0
	}
	if len(ds.Data) == 0 || len(ds.Data[0]) <= idx {
		return 0
	}

	val, err := toFloat(ds.Data[0][idx])
	if err != nil {
		return 0
	}
	return int(val)
}

func toFloat(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
	case json.Number:
		return v.Float64()
	default:
		return 0, fmt.Errorf("unsupported numeric type %T", value)
	}
}
