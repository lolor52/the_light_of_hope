package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const issBaseURL = "https://iss.moex.com/iss"

// ISSClient предоставляет методы обращения к MOEX ISS.
type ISSClient struct {
	passport *PassportClient
}

// BoardMetadata содержит параметры площадки, необходимые для построения URL.
type BoardMetadata struct {
	Engine string
	Market string
}

// Trade описывает отдельную сделку из ISS.
type Trade struct {
	Price          float64
	Quantity       float64
	Value          float64
	TradingSession string
	TradeTime      string
}

// NewISSClient создаёт клиента ISS на базе авторизованного Passport клиента.
func NewISSClient(passport *PassportClient) *ISSClient {
	return &ISSClient{passport: passport}
}

// BoardMetadata загружает параметры движка и рынка по идентификатору площадки.
func (c *ISSClient) BoardMetadata(ctx context.Context, boardID string) (BoardMetadata, error) {
	endpoint := fmt.Sprintf("%s/boards/%s.json", issBaseURL, url.PathEscape(strings.ToUpper(boardID)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return BoardMetadata{}, fmt.Errorf("moex iss board request: %w", err)
	}

	resp, err := c.passport.Do(ctx, req)
	if err != nil {
		return BoardMetadata{}, fmt.Errorf("moex iss board call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return BoardMetadata{}, fmt.Errorf("moex iss board status: %s", resp.Status)
	}

	var payload struct {
		Boards issTable `json:"boards"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return BoardMetadata{}, fmt.Errorf("moex iss board decode: %w", err)
	}
	if len(payload.Boards.Data) == 0 {
		return BoardMetadata{}, fmt.Errorf("moex iss board: board %s not found", boardID)
	}

	columns := payload.Boards.columnsIndex()
	engine, err := payload.Boards.valueString(columns, "engine", 0)
	if err != nil {
		return BoardMetadata{}, err
	}
	market, err := payload.Boards.valueString(columns, "market", 0)
	if err != nil {
		return BoardMetadata{}, err
	}

	return BoardMetadata{
		Engine: engine,
		Market: market,
	}, nil
}

// Trades возвращает сделки за указанный день.
func (c *ISSClient) Trades(ctx context.Context, meta BoardMetadata, boardID, secID string, sessionDate time.Time) ([]Trade, error) {
	var result []Trade
	start := 0
	dateStr := sessionDate.Format("2006-01-02")

	for {
		endpoint := fmt.Sprintf("%s/engines/%s/markets/%s/boards/%s/securities/%s/trades.json",
			issBaseURL,
			url.PathEscape(strings.ToLower(meta.Engine)),
			url.PathEscape(strings.ToLower(meta.Market)),
			url.PathEscape(strings.ToUpper(boardID)),
			url.PathEscape(strings.ToUpper(secID)),
		)

		values := url.Values{}
		values.Set("from", dateStr)
		values.Set("till", dateStr)
		values.Set("start", strconv.Itoa(start))
		values.Set("iss.meta", "off")
		values.Set("iss.json", "extended")

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+values.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("moex iss trades request: %w", err)
		}

		resp, err := c.passport.Do(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("moex iss trades call: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("moex iss trades status: %s", resp.Status)
		}

		var payload struct {
			Trades       issTable `json:"trades"`
			TradesCursor issTable `json:"trades.cursor"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("moex iss trades decode: %w", err)
		}
		resp.Body.Close()

		columns := payload.Trades.columnsIndex()
		for i := range payload.Trades.Data {
			row := payload.Trades.Data[i]
			price, err := payload.Trades.valueFloat(columns, "price", i)
			if err != nil {
				return nil, err
			}
			quantity, err := payload.Trades.valueFloat(columns, "quantity", i)
			if err != nil {
				return nil, err
			}
			value, err := payload.Trades.valueFloat(columns, "value", i)
			if err != nil {
				value = price * quantity
			}
			session := payload.Trades.valueString(columns, "tradingsession", i)
			tradeTime, _ := payload.Trades.valueString(columns, "tradetime", i)

			result = append(result, Trade{
				Price:          price,
				Quantity:       quantity,
				Value:          value,
				TradingSession: session,
				TradeTime:      tradeTime,
			})
		}

		nextStart, total, err := parseCursor(payload.TradesCursor)
		if err != nil {
			return nil, err
		}

		if total == 0 {
			break
		}

		start = nextStart
		if start >= total {
			break
		}
	}

	return result, nil
}

type issTable struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
}

func (t issTable) columnsIndex() map[string]int {
	idx := make(map[string]int, len(t.Columns))
	for i, column := range t.Columns {
		idx[strings.ToLower(column)] = i
	}
	return idx
}

func (t issTable) valueString(idx map[string]int, name string, row int) (string, error) {
	position, ok := idx[strings.ToLower(name)]
	if !ok {
		return "", fmt.Errorf("moex iss: column %s not found", name)
	}
	if row >= len(t.Data) || position >= len(t.Data[row]) {
		return "", fmt.Errorf("moex iss: invalid row for column %s", name)
	}
	switch v := t.Data[row][position].(type) {
	case string:
		return v, nil
	case nil:
		return "", nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func (t issTable) valueFloat(idx map[string]int, name string, row int) (float64, error) {
	position, ok := idx[strings.ToLower(name)]
	if !ok {
		return 0, fmt.Errorf("moex iss: column %s not found", name)
	}
	if row >= len(t.Data) || position >= len(t.Data[row]) {
		return 0, fmt.Errorf("moex iss: invalid row for column %s", name)
	}
	switch v := t.Data[row][position].(type) {
	case float64:
		return v, nil
	case string:
		if v == "" {
			return 0, fmt.Errorf("moex iss: empty value for column %s", name)
		}
		f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
		if err != nil {
			return 0, fmt.Errorf("moex iss: parse %s: %w", name, err)
		}
		return f, nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("moex iss: unexpected type %T for column %s", v, name)
	}
}

func parseCursor(table issTable) (int, int, error) {
	if len(table.Data) == 0 {
		return 0, 0, nil
	}

	idx := table.columnsIndex()
	indexVal, err := table.valueFloat(idx, "index", 0)
	if err != nil {
		return 0, 0, err
	}
	totalVal, err := table.valueFloat(idx, "total", 0)
	if err != nil {
		return 0, 0, err
	}
	pageSizeVal, err := table.valueFloat(idx, "pagesize", 0)
	if err != nil {
		pageSizeVal = float64(len(table.Data))
	}

	nextStart := int(indexVal + pageSizeVal)
	return nextStart, int(totalVal), nil
}

// BuildTradesURL конструирует относительный путь для публичного использования.
func BuildTradesURL(meta BoardMetadata, boardID, secID string) string {
	return path.Join(
		"engines",
		strings.ToLower(meta.Engine),
		"markets",
		strings.ToLower(meta.Market),
		"boards",
		strings.ToUpper(boardID),
		"securities",
		strings.ToUpper(secID),
		"trades",
	)
}
