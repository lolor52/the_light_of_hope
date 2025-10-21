package moex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	passportAuthEndpoint = "https://passport.moex.com/authenticate"
	baseIssURL           = "https://iss.moex.com/iss"
)

// Client инкапсулирует обращение к MOEX ISS c авторизацией через Passport.
type Client struct {
	httpClient *http.Client
	login      string
	password   string
}

// NewClient выполняет авторизацию и возвращает готовый клиент MOEX ISS.
func NewClient(ctx context.Context, login, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	httpClient := &http.Client{
		Timeout:   15 * time.Second,
		Transport: http.DefaultTransport,
		Jar:       jar,
	}

	client := &Client{
		httpClient: httpClient,
		login:      login,
		password:   password,
	}

	if err := client.authenticate(ctx); err != nil {
		return nil, err
	}

	return client, nil
}

func (c *Client) authenticate(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, passportAuthEndpoint, nil)
	if err != nil {
		return fmt.Errorf("create auth request: %w", err)
	}

	req.SetBasicAuth(c.login, c.password)
	q := req.URL.Query()
	q.Set("lang", "ru")
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("passport auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("passport auth unexpected status: %s", resp.Status)
	}

	// Ответ в JSON, проверим, что авторизация успешна.
	var payload struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode passport auth response: %w", err)
	}
	if !payload.Success {
		if payload.Error == "" {
			payload.Error = "unknown passport error"
		}
		return errors.New(payload.Error)
	}

	return nil
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
