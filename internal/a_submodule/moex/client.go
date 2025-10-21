package moex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"invest_intraday/internal/auth/moex_passport"
)

const baseIssURL = "https://iss.moex.com/iss"

// Client инкапсулирует обращение к MOEX ISS c авторизацией через Passport.
type Client struct {
	httpClient *http.Client
}

// NewClient выполняет авторизацию и возвращает готовый клиент MOEX ISS.
func NewClient(ctx context.Context, login, password string) (*Client, error) {
	session, err := moexpassport.NewSession(ctx, login, password)
	if err != nil {
		return nil, fmt.Errorf("create moex passport session: %w", err)
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request unexpected status: %s", resp.Status)
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
