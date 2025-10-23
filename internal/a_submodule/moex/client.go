package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	moexpassport "invest_intraday/internal/auth/moex_passport"
)

const baseIssURL = "https://iss.moex.com/iss"

// Client инкапсулирует обращение к MOEX ISS c авторизацией через Passport.
type Client struct {
	httpClient *http.Client
}

// NewClient выполняет авторизацию и возвращает готовый клиент MOEX ISS.
func NewClient(ctx context.Context, login, password string) (*Client, error) {
	session, err := moexpassport.Authenticate(ctx, login, password)
	if err != nil {
		return nil, fmt.Errorf("authenticate via moex passport: %w", err)
	}

	return &Client{
		httpClient: session.HTTPClient(),
	}, nil
}

// getJSON выполняет GET-запрос и декодирует JSON-ответ.
func (c *Client) getJSON(ctx context.Context, endpoint string, query url.Values, target interface{}) error {
	u := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseIssURL, "/"), strings.TrimPrefix(endpoint, "/"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if len(query) > 0 {
		req.URL.RawQuery = query.Encode()
	}

	log.Printf("tickers_filling: запрос к MOEX ISS: %s %s", req.Method, req.URL.String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	log.Printf("tickers_filling: ответ MOEX ISS (%s): %s", req.URL.String(), string(body))

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
