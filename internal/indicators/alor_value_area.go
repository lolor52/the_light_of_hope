package indicators

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"invest_intraday/internal/a_technical/db"
	"invest_intraday/internal/auth/alor"
)

const (
	moscowLocationName   = "Europe/Moscow"
	mainSessionStartHour = 10
	mainSessionEndHour   = 18
	mainSessionEndMinute = 45
	valueAreaFraction    = 0.7
	priceScale           = 100000
	exchangeMOEX         = "MOEX"
	exchangeSPBX         = "SPBX"
)

// ErrNoTrades означает, что для указанной сессии не найдено сделок.
var ErrNoTrades = errors.New("indicators: no trades in main session")

// ErrInvalidRequest означает, что Alor вернул ошибку из-за некорректных параметров запроса.
var ErrInvalidRequest = errors.New("indicators: invalid request parameters")

// TokenProvider описывает зависимость, способную предоставить access-token Alor.
type TokenProvider interface {
	AccessToken(ctx context.Context) (string, error)
}

// MarketDataClient инкапсулирует запрос исторических сделок к Alor.
type MarketDataClient struct {
	baseURL       string
	httpClient    *http.Client
	tokenProvider TokenProvider
	lastResponse  string
}

// NewMarketDataClient создаёт клиент для запросов маркет-даты Alor.
func NewMarketDataClient(baseURL string, tokenProvider TokenProvider) *MarketDataClient {
	base := strings.TrimRight(baseURL, "/")
	client := &MarketDataClient{
		baseURL:       base,
		httpClient:    http.DefaultClient,
		tokenProvider: tokenProvider,
	}

	return client
}

// WithHTTPClient переопределяет HTTP-клиент для запросов маркет-даты.
func (c *MarketDataClient) WithHTTPClient(httpClient *http.Client) {
	if httpClient != nil {
		c.httpClient = httpClient
	}
}

// trade описывает агрегированные данные об одной сделке.
type trade struct {
	Price  float64
	Volume float64
}

// ValueAreaCalculator рассчитывает VWAP, VAL и VAH для основной сессии.
type ValueAreaCalculator struct {
	tickerInfos *db.TickerInfoRepository
	mdClient    *MarketDataClient
}

// NewValueAreaCalculator создаёт вычислитель индикаторов.
func NewValueAreaCalculator(repo *db.TickerInfoRepository, mdClient *MarketDataClient) *ValueAreaCalculator {
	return &ValueAreaCalculator{
		tickerInfos: repo,
		mdClient:    mdClient,
	}
}

// SessionProfile содержит рассчитанные показатели основной торговой сессии.
type SessionProfile struct {
	VWAP float64
	VAL  float64
	VAH  float64
}

// CalculateMainSessionProfile возвращает VWAP, VAL и VAH для основной торговой сессии тикера.
func (c *ValueAreaCalculator) CalculateMainSessionProfile(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (SessionProfile, error) {
	if c == nil {
		return SessionProfile{}, errors.New("indicators: nil calculator")
	}
	if c.tickerInfos == nil {
		return SessionProfile{}, errors.New("indicators: ticker repository is required")
	}
	if c.mdClient == nil {
		return SessionProfile{}, errors.New("indicators: market data client is required")
	}

	info, err := c.tickerInfos.GetByID(ctx, tickerInfoID)
	if err != nil {
		return SessionProfile{}, fmt.Errorf("load ticker info: %w", err)
	}

	start, end, err := mainSessionBounds(sessionDate)
	if err != nil {
		return SessionProfile{}, fmt.Errorf("detect session bounds: %w", err)
	}

	trades, err := c.mdClient.FetchTrades(ctx, info.BoardID, info.SecID, start, end)
	if err != nil {
		return SessionProfile{}, err
	}

	vwap, err := calcVWAP(trades)
	if err != nil {
		return SessionProfile{}, err
	}

	val, vah, err := calcValueArea(trades)
	if err != nil {
		return SessionProfile{}, err
	}

	return SessionProfile{VWAP: vwap, VAL: val, VAH: vah}, nil
}

// LastAlorResponse возвращает текст последнего ответа Alor.
func (c *ValueAreaCalculator) LastAlorResponse() string {
	if c == nil || c.mdClient == nil {
		return ""
	}

	return c.mdClient.LastResponse()
}

// FetchTrades выгружает сделки за указанный период у Alor.
func (c *MarketDataClient) FetchTrades(ctx context.Context, instrumentGroup, secID string, from, to time.Time) ([]trade, error) {
	if c == nil {
		return nil, errors.New("indicators: nil market data client")
	}
	c.lastResponse = ""
	if c.tokenProvider == nil {
		return nil, errors.New("indicators: token provider is required")
	}
	if c.httpClient == nil {
		return nil, errors.New("indicators: http client is required")
	}
	if strings.TrimSpace(instrumentGroup) == "" {
		return nil, errors.New("indicators: instrument group is required")
	}
	if strings.TrimSpace(secID) == "" {
		return nil, errors.New("indicators: secid is required")
	}
	if !to.After(from) {
		return nil, errors.New("indicators: invalid time range")
	}

	token, err := c.tokenProvider.AccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtain access token: %w", err)
	}

	exchange := detectExchange(instrumentGroup)
	endpoint := fmt.Sprintf("%s/md/v2/Securities/%s/%s/alltrades", c.baseURL, url.PathEscape(exchange), url.PathEscape(secID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	q := req.URL.Query()
	q.Set("from", formatUnixSeconds(from))
	q.Set("to", formatUnixSeconds(to))
	q.Set("instrumentGroup", instrumentGroup)
	q.Set("token", token)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.lastResponse = fmt.Sprintf("request error %s %s: %v", req.Method, req.URL.String(), err)
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.lastResponse = fmt.Sprintf("read body error: %v", err)
		return nil, fmt.Errorf("read response: %w", err)
	}
	c.lastResponse = formatResponseSnapshot(req.Method, req.URL, resp.StatusCode, body)

	switch resp.StatusCode {
	case http.StatusOK:
		if len(bytes.TrimSpace(body)) == 0 {
			return nil, ErrNoTrades
		}
	case http.StatusNoContent:
		return nil, ErrNoTrades
	case http.StatusBadRequest, http.StatusNotFound:
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("alor market data: invalid request status %d: %s: %w", resp.StatusCode, message, ErrInvalidRequest)
	default:
		return nil, fmt.Errorf("alor market data: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload []map[string]any
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrNoTrades
		}
		return nil, fmt.Errorf("decode response: %w", err)
	}

	trades := make([]trade, 0, len(payload))
	for _, item := range payload {
		price, _ := extractFloat(item, "price", "Price", "lastPrice", "p")
		volume, volumeOK := extractFloat(item, "qty", "volume", "Vol", "quantity")
		if !volumeOK {
			if value, ok := extractFloat(item, "value", "Value"); ok && price > 0 {
				volume = value / price
			}
		}

		if price <= 0 || volume <= 0 {
			continue
		}

		trades = append(trades, trade{Price: price, Volume: volume})
	}

	if len(trades) == 0 {
		return nil, ErrNoTrades
	}

	return trades, nil
}

func detectExchange(instrumentGroup string) string {
	group := strings.ToUpper(strings.TrimSpace(instrumentGroup))
	if strings.HasPrefix(group, "SPB") {
		return exchangeSPBX
	}

	return exchangeMOEX
}

func formatUnixSeconds(t time.Time) string {
	return strconv.FormatInt(t.UTC().Unix(), 10)
}

// LastResponse возвращает строку для логирования последнего ответа Alor.
func (c *MarketDataClient) LastResponse() string {
	if c == nil {
		return ""
	}
	return c.lastResponse
}

func truncateForLog(data []byte) string {
	const maxLen = 512
	text := strings.TrimSpace(string(data))
	if len(text) <= maxLen {
		return text
	}

	return text[:maxLen] + "..."
}

func formatResponseSnapshot(method string, u *url.URL, status int, body []byte) string {
	text := truncateForLog(body)
	if text == "" {
		text = "<empty>"
	}

	return fmt.Sprintf("%s %s status=%d body=%s", method, u.String(), status, text)
}

func extractFloat(item map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := item[key]
		if !ok || value == nil {
			continue
		}

		switch v := value.(type) {
		case float64:
			return v, true
		case json.Number:
			f, err := v.Float64()
			if err == nil {
				return f, true
			}
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err == nil {
				return f, true
			}
		case int:
			return float64(v), true
		case int64:
			return float64(v), true
		case uint64:
			return float64(v), true
		}
	}

	return 0, false
}

func calcVWAP(trades []trade) (float64, error) {
	var totalVolume, weightedPrice float64
	for _, t := range trades {
		if t.Price <= 0 || t.Volume <= 0 {
			continue
		}
		totalVolume += t.Volume
		weightedPrice += t.Price * t.Volume
	}

	if totalVolume == 0 {
		return 0, ErrNoTrades
	}

	return weightedPrice / totalVolume, nil
}

type priceBucket struct {
	Price  float64
	Volume float64
}

func calcValueArea(trades []trade) (float64, float64, error) {
	buckets := aggregateVolume(trades)
	if len(buckets) == 0 {
		return 0, 0, ErrNoTrades
	}

	var totalVolume float64
	pocIndex := 0
	maxVolume := buckets[0].Volume
	for i, bucket := range buckets {
		totalVolume += bucket.Volume
		if bucket.Volume > maxVolume {
			maxVolume = bucket.Volume
			pocIndex = i
		}
	}

	if totalVolume == 0 {
		return 0, 0, ErrNoTrades
	}

	targetVolume := totalVolume * valueAreaFraction
	lower, upper := pocIndex, pocIndex
	areaVolume := buckets[pocIndex].Volume
	nextLower := lower - 1
	nextUpper := upper + 1

	for areaVolume < targetVolume {
		var lowerVolume, upperVolume float64
		if nextLower >= 0 {
			lowerVolume = buckets[nextLower].Volume
		} else {
			lowerVolume = -1
		}

		if nextUpper < len(buckets) {
			upperVolume = buckets[nextUpper].Volume
		} else {
			upperVolume = -1
		}

		if lowerVolume < 0 && upperVolume < 0 {
			break
		}

		takeLower := false
		switch {
		case lowerVolume < 0:
			takeLower = false
		case upperVolume < 0:
			takeLower = true
		default:
			takeLower = lowerVolume >= upperVolume
		}

		if takeLower {
			lower = nextLower
			areaVolume += buckets[lower].Volume
			nextLower = lower - 1
		} else {
			upper = nextUpper
			areaVolume += buckets[upper].Volume
			nextUpper = upper + 1
		}
	}

	return buckets[lower].Price, buckets[upper].Price, nil
}

func aggregateVolume(trades []trade) []priceBucket {
	volumes := make(map[int64]float64)
	for _, t := range trades {
		if t.Price <= 0 || t.Volume <= 0 {
			continue
		}
		key := int64(math.Round(t.Price * priceScale))
		volumes[key] += t.Volume
	}

	buckets := make([]priceBucket, 0, len(volumes))
	for key, volume := range volumes {
		buckets = append(buckets, priceBucket{
			Price:  float64(key) / priceScale,
			Volume: volume,
		})
	}

	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Price == buckets[j].Price {
			return i < j
		}
		return buckets[i].Price < buckets[j].Price
	})

	return buckets
}

func mainSessionBounds(date time.Time) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(moscowLocationName)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("load moscow location: %w", err)
	}

	localDate := date.In(loc)
	start := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), mainSessionStartHour, 0, 0, 0, loc)
	end := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), mainSessionEndHour, mainSessionEndMinute, 0, 0, loc)

	return start, end, nil
}

// Compile-time проверка совместимости клиента авторизации с интерфейсом токена.
var _ TokenProvider = (*alor.Client)(nil)
