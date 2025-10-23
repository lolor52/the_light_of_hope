package moex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)

const (
	passportURL      = "https://passport.moex.com/authenticate"
	defaultUserAgent = "invest-intraday-bot/1.0"
)

// Credentials содержит данные для авторизации в MOEX Passport.
type Credentials struct {
	Login    string
	Password string
}

// PassportClient выполняет авторизацию и хранит cookies для дальнейших запросов.
type PassportClient struct {
	client        *http.Client
	credentials   Credentials
	authOnce      sync.Once
	authErr       error
	authMu        sync.Mutex
	authenticated bool
}

// NewPassportClient создаёт клиента Passport с cookie jar и таймаутом запросов.
func NewPassportClient(creds Credentials) (*PassportClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		Jar:     jar,
	}

	return &PassportClient{
		client:      httpClient,
		credentials: creds,
	}, nil
}

// Do выполняет HTTP-запрос, гарантируя прохождение авторизации заранее.
func (c *PassportClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)
	if ua := req.Header.Get("User-Agent"); ua == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}

	return c.client.Do(req)
}

func (c *PassportClient) ensureAuthenticated(ctx context.Context) error {
	c.authMu.Lock()
	authenticated := c.authenticated
	c.authMu.Unlock()
	if authenticated {
		return nil
	}

	c.authOnce.Do(func() {
		c.authErr = c.authenticate(ctx)
		if c.authErr == nil {
			c.authMu.Lock()
			c.authenticated = true
			c.authMu.Unlock()
		}
	})

	if c.authErr != nil {
		return c.authErr
	}

	return nil
}

func (c *PassportClient) authenticate(ctx context.Context) error {
	if c.credentials.Login == "" || c.credentials.Password == "" {
		return fmt.Errorf("moex passport: empty credentials")
	}

	payload := map[string]string{
		"login":    c.credentials.Login,
		"password": c.credentials.Password,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("moex passport: encode payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, passportURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("moex passport: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("moex passport: send request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("moex passport: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("moex passport: status %d: %s", resp.StatusCode, bytes.TrimSpace(data))
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("moex passport: decode response: %w", err)
	}
	if !result.Success {
		if result.Error == "" {
			result.Error = "unknown error"
		}
		return fmt.Errorf("moex passport: %s", result.Error)
	}

	return nil
}
