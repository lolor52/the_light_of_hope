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
	if req == nil {
		return nil, fmt.Errorf("moex passport: request is nil")
	}

	req = req.Clone(ctx)
	if ua := req.Header.Get("User-Agent"); ua == "" {
		req.Header.Set("User-Agent", defaultUserAgent)
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}

	if shouldReauthenticate(resp.StatusCode) {
		resp.Body.Close()

		if err := c.reauthenticate(ctx); err != nil {
			return nil, err
		}

		retryReq, err := cloneRequest(req)
		if err != nil {
			return nil, err
		}

		return c.do(ctx, retryReq)
	}

	return resp, nil
}

func (c *PassportClient) ensureAuthenticated(ctx context.Context) error {
	c.authMu.Lock()
	if c.authenticated {
		c.authMu.Unlock()
		return nil
	}
	c.authMu.Unlock()

	return c.authenticate(ctx)
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

	c.authMu.Lock()
	c.authenticated = true
	c.authMu.Unlock()

	return nil
}

func (c *PassportClient) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := c.ensureAuthenticated(ctx); err != nil {
		return nil, err
	}

	return c.client.Do(req)
}

func (c *PassportClient) reauthenticate(ctx context.Context) error {
	c.authMu.Lock()
	c.authenticated = false
	c.authMu.Unlock()

	return c.ensureAuthenticated(ctx)
}

func shouldReauthenticate(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		return nil, fmt.Errorf("moex passport: cannot replay request with non-rewindable body")
	}

	clone := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		clone.Body = req.Body
		return clone, nil
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("moex passport: clone body: %w", err)
	}
	clone.Body = body

	return clone, nil
}
