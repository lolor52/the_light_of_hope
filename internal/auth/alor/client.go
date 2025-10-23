package alor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Environment описывает набор базовых URL для Alor.
//
// ProdEnvironment и DevEnvironment покрывают основные контуры API.
type Environment struct {
	OAuthURL string
	APIURL   string
}

// ProdEnvironment содержит URL для боевого контура Alor.
var ProdEnvironment = Environment{
	OAuthURL: "https://oauth.alor.ru",
	APIURL:   "https://api.alor.ru",
}

// DevEnvironment содержит URL для тестового контура Alor.
var DevEnvironment = Environment{
	OAuthURL: "https://oauthdev.alor.ru",
	APIURL:   "https://apidev.alor.ru",
}

// AuthorizationChecker описывает функциональность проверки авторизации в Alor.
type AuthorizationChecker interface {
	CheckAuthorization(ctx context.Context) error
}

// Client реализует логику работы с Alor API через access-token.
type Client struct {
	httpClient    *http.Client
	refreshToken  string
	env           Environment
	refreshBefore time.Duration

	mu    sync.RWMutex
	token *accessToken
}

type accessToken struct {
	value     string
	expiresAt time.Time
}

// ErrUnauthorized возвращается, если Alor отклоняет авторизацию даже после обновления access-token.
var ErrUnauthorized = errors.New("alor: unauthorized")

const defaultRefreshBefore = 2 * time.Minute

// Option задаёт дополнительные параметры клиента Alor.
type Option func(*Client)

// WithHTTPClient позволяет указать кастомный http.Client.
func WithHTTPClient(c *http.Client) Option {
	return func(client *Client) {
		client.httpClient = c
	}
}

// WithEnvironment переопределяет используемые базовые URL Alor.
func WithEnvironment(env Environment) Option {
	return func(client *Client) {
		client.env = env
	}
}

// WithRefreshMargin позволяет задать период, за который токен должен освежаться до истечения.
func WithRefreshMargin(d time.Duration) Option {
	return func(client *Client) {
		if d > 0 {
			client.refreshBefore = d
		}
	}
}

// NewClient создаёт клиента авторизации Alor с использованием refresh-token.
func NewClient(refreshToken string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("alor: refresh token is required")
	}

	client := &Client{
		refreshToken:  refreshToken,
		env:           ProdEnvironment,
		refreshBefore: defaultRefreshBefore,
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.httpClient == nil {
		client.httpClient = http.DefaultClient
	}

	return client, nil
}

// CheckAuthorization запрашивает приватный ресурс Alor, предварительно обеспечивая валидный access-token.
// При получении 401/403 происходит однократное обновление токена.
func (c *Client) CheckAuthorization(ctx context.Context) error {
	if c == nil {
		return errors.New("alor: nil client")
	}

	token, err := c.validAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	authorized, needsRefresh, err := c.queryOrderBook(ctx, token)
	if err != nil {
		return err
	}

	if authorized {
		return nil
	}

	if needsRefresh {
		token, err = c.refreshAccessToken(ctx, true)
		if err != nil {
			return fmt.Errorf("refresh access token: %w", err)
		}

		authorized, _, err = c.queryOrderBook(ctx, token)
		if err != nil {
			return err
		}

		if authorized {
			return nil
		}
	}

	return ErrUnauthorized
}

// AccessToken возвращает действующий access-token Alor, автоматически обновляя его при необходимости.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	if c == nil {
		return "", errors.New("alor: nil client")
	}

	token, err := c.validAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get access token: %w", err)
	}

	return token, nil
}

func (c *Client) validAccessToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	cached := c.token
	c.mu.RUnlock()

	if cached != nil && time.Until(cached.expiresAt) > c.refreshBefore {
		return cached.value, nil
	}

	return c.refreshAccessToken(ctx, false)
}

func (c *Client) refreshAccessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !force && c.token != nil && time.Until(c.token.expiresAt) > c.refreshBefore {
		return c.token.value, nil
	}

	value, expiresAt, err := c.requestAccessToken(ctx)
	if err != nil {
		return "", err
	}

	c.token = &accessToken{value: value, expiresAt: expiresAt}
	return value, nil
}

func (c *Client) requestAccessToken(ctx context.Context) (string, time.Time, error) {
	refreshURL := strings.TrimRight(c.env.OAuthURL, "/") + "/refresh"
	reqURL, err := url.Parse(refreshURL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("build refresh url: %w", err)
	}

	query := reqURL.Query()
	query.Set("token", c.refreshToken)
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("request access token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", time.Time{}, fmt.Errorf("unexpected refresh status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed refreshResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", time.Time{}, fmt.Errorf("decode refresh response: %w", err)
	}

	token := parsed.AccessToken
	if token == "" {
		token = parsed.AccessTokenLower
	}

	if token == "" {
		return "", time.Time{}, errors.New("alor: empty access token in response")
	}

	expiresAt, err := parseJWTExpiration(token)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse token expiration: %w", err)
	}

	return token, expiresAt, nil
}

type refreshResponse struct {
	AccessToken      string `json:"AccessToken"`
	AccessTokenLower string `json:"access_token"`
}

func (c *Client) queryOrderBook(ctx context.Context, token string) (bool, bool, error) {
	if strings.TrimSpace(token) == "" {
		return false, false, errors.New("alor: empty access token")
	}

	endpoint := strings.TrimRight(c.env.APIURL, "/") + "/md/v2/orderbooks/MOEX/SBER?depth=1&format=Simple"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, false, fmt.Errorf("create orderbook request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, false, fmt.Errorf("request orderbook: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var raw json.RawMessage
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
			return false, false, fmt.Errorf("decode orderbook response: %w", err)
		}
		return true, false, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		io.Copy(io.Discard, resp.Body)
		return false, true, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return false, false, fmt.Errorf("unexpected orderbook status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func parseJWTExpiration(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, errors.New("alor: malformed JWT")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("decode payload: %w", err)
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("decode claims: %w", err)
	}

	expValue, ok := claims["exp"].(float64)
	if !ok {
		return time.Time{}, errors.New("alor: exp claim missing")
	}

	return time.Unix(int64(expValue), 0), nil
}
