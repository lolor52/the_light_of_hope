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

const (
	productionOAuthURL = "https://oauth.alor.ru"
	productionAPIURL   = "https://api.alor.ru"

	tokenRefreshMargin = 2 * time.Minute
	requestTimeout     = 30 * time.Second
)

// Option задаёт дополнительные параметры клиента Alor.
type Option func(*Client)

// WithHTTPClient позволяет переопределить HTTP-клиент, используемый для запросов к API.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithBaseURLs задаёт базовые URL для OAuth и API.
func WithBaseURLs(oauthURL, apiURL string) Option {
	return func(c *Client) {
		c.oauthBaseURL = strings.TrimRight(oauthURL, "/")
		c.apiBaseURL = strings.TrimRight(apiURL, "/")
	}
}

// Client инкапсулирует работу с OAuth и приватными запросами Alor API.
type Client struct {
	httpClient *http.Client

	mu sync.Mutex

	refreshToken string
	accessToken  string

	accessTokenExpiry time.Time

	oauthBaseURL string
	apiBaseURL   string
}

// NewClient создаёт новый экземпляр клиента Alor.
func NewClient(refreshToken string, opts ...Option) (*Client, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, errors.New("alor: пустой refresh-токен")
	}

	client := &Client{
		httpClient:   &http.Client{Timeout: requestTimeout},
		refreshToken: refreshToken,
		oauthBaseURL: productionOAuthURL,
		apiBaseURL:   productionAPIURL,
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.httpClient == nil {
		client.httpClient = &http.Client{Timeout: requestTimeout}
	}
	if client.oauthBaseURL == "" {
		client.oauthBaseURL = productionOAuthURL
	}
	if client.apiBaseURL == "" {
		client.apiBaseURL = productionAPIURL
	}

	return client, nil
}

// Status содержит информацию об актуальности авторизации.
type Status struct {
	Authorized bool      `json:"authorized"`
	ExpiresAt  time.Time `json:"expires_at"`
	Message    string    `json:"message,omitempty"`
}

// CheckAuthorization выполняет приватный запрос к Alor API и возвращает статус авторизации.
func (c *Client) CheckAuthorization(ctx context.Context) (Status, error) {
	var status Status

	resp, err := c.performProtectedRequest(ctx)
	if err != nil {
		return status, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return status, fmt.Errorf("не удалось прочитать ответ приватного эндпоинта: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		if !json.Valid(body) {
			return status, errors.New("получен некорректный JSON от приватного эндпоинта")
		}
		status.Authorized = true
	case http.StatusUnauthorized, http.StatusForbidden:
		status.Message = "некорректный или просроченный access-токен"
	default:
		status.Message = fmt.Sprintf("приватный эндпоинт вернул код %d", resp.StatusCode)
		trimmed := strings.TrimSpace(string(body))
		if trimmed != "" {
			status.Message = fmt.Sprintf("%s: %s", status.Message, trimmed)
		}
	}

	status.ExpiresAt = c.currentAccessTokenExpiry()

	return status, nil
}

func (c *Client) performProtectedRequest(ctx context.Context) (*http.Response, error) {
	token, err := c.obtainAccessToken(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить access-токен: %w", err)
	}

	resp, err := c.sendOrderbookRequest(ctx, token)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}

	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	token, err = c.obtainAccessToken(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("не удалось обновить access-токен: %w", err)
	}

	return c.sendOrderbookRequest(ctx, token)
}

func (c *Client) sendOrderbookRequest(ctx context.Context, token string) (*http.Response, error) {
	endpoint := c.apiBaseURL + "/md/v2/orderbooks/MOEX/SBER?depth=1&format=Simple"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("не удалось создать запрос к приватному эндпоинту: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("запрос к приватному эндпоинту завершился ошибкой: %w", err)
	}

	return resp, nil
}

func (c *Client) obtainAccessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	token := c.accessToken
	expiry := c.accessTokenExpiry
	refreshToken := c.refreshToken
	c.mu.Unlock()

	if !force && token != "" && time.Until(expiry) > tokenRefreshMargin {
		return token, nil
	}

	if refreshToken == "" {
		return "", errors.New("alor: отсутствует refresh-токен")
	}

	newToken, newExpiry, newRefresh, err := c.requestAccessToken(ctx, refreshToken)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.accessToken = newToken
	c.accessTokenExpiry = newExpiry
	if newRefresh != "" {
		c.refreshToken = newRefresh
	}

	return newToken, nil
}

func (c *Client) requestAccessToken(ctx context.Context, refreshToken string) (string, time.Time, string, error) {
	endpoint := c.oauthBaseURL + "/refresh"

	form := url.Values{}
	form.Set("token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("не удалось создать запрос на обновление access-токена: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("ошибка обращения к OAuth эндпоинту: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		limited := io.LimitReader(resp.Body, 1024)
		body, _ := io.ReadAll(limited)
		return "", time.Time{}, "", fmt.Errorf("OAuth эндпоинт вернул код %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", time.Time{}, "", fmt.Errorf("не удалось декодировать ответ OAuth эндпоинта: %w", err)
	}

	accessToken := pickString(data, "AccessToken", "accessToken")
	if accessToken == "" {
		return "", time.Time{}, "", errors.New("OAuth эндпоинт не вернул access-токен")
	}

	expiry, err := parseJWTExpiry(accessToken)
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("не удалось определить срок жизни access-токена: %w", err)
	}

	refresh := pickString(data, "RefreshToken", "refreshToken")

	return accessToken, expiry, refresh, nil
}

func pickString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			if str, ok := value.(string); ok {
				return str
			}
		}
	}
	return ""
}

func parseJWTExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, errors.New("некорректный формат JWT")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("не удалось декодировать payload JWT: %w", err)
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, fmt.Errorf("не удалось разобрать payload JWT: %w", err)
	}
	if claims.Exp == 0 {
		return time.Time{}, errors.New("в JWT отсутствует поле exp")
	}

	return time.Unix(claims.Exp, 0).UTC(), nil
}

func (c *Client) currentAccessTokenExpiry() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accessTokenExpiry
}
