package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	alorAuthEndpoint     = "https://oauth.alor.ru/refresh"
	authRequestTimeout   = 15 * time.Second
	defaultTokenLifetime = 30 * time.Minute
	tokenRefreshMargin   = 30 * time.Second
)

// AlorTokenProvider отвечает за получение и кеширование access-токена для API АЛОР.
type AlorTokenProvider struct {
	refreshToken string
	httpClient   *http.Client
	endpoint     string

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// NewAlorTokenProvider создаёт провайдер access-токенов по refresh-токену.
func NewAlorTokenProvider(refreshToken string) (*AlorTokenProvider, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("auth: пустой refresh-токен")
	}

	provider := &AlorTokenProvider{
		refreshToken: refreshToken,
		httpClient: &http.Client{
			Timeout: authRequestTimeout,
		},
		endpoint: alorAuthEndpoint,
	}

	return provider, nil
}

// AccessToken возвращает действующий access-токен, при необходимости обновляя его.
func (p *AlorTokenProvider) AccessToken(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", errors.New("auth: контекст не может быть nil")
	}

	token, valid := p.cachedToken()
	if valid {
		return token, nil
	}

	return p.refreshTokenValue(ctx)
}

// Invalidate помечает кешированный access-токен как недействительный.
func (p *AlorTokenProvider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token = ""
	p.expiresAt = time.Time{}
}

func (p *AlorTokenProvider) cachedToken() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.token == "" {
		return "", false
	}

	now := time.Now()
	if now.After(p.expiresAt.Add(-tokenRefreshMargin)) {
		return "", false
	}

	return p.token, true
}

func (p *AlorTokenProvider) refreshTokenValue(ctx context.Context) (string, error) {
	token, expiresAt, err := p.requestAccessToken(ctx)
	if err != nil {
		return "", err
	}

	p.mu.Lock()
	p.token = token
	p.expiresAt = expiresAt
	p.mu.Unlock()

	return token, nil
}

func (p *AlorTokenProvider) requestAccessToken(ctx context.Context) (string, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: создание запроса обновления: %w", err)
	}

	query := url.Values{}
	query.Set("token", p.refreshToken)
	req.URL.RawQuery = query.Encode()

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: отправка запроса обновления: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("auth: обновление токена завершилось статусом %s", resp.Status)
	}

	var payload refreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: декодирование ответа при обновлении: %w", err)
	}

	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", time.Time{}, fmt.Errorf("auth: сервер не вернул access-токен")
	}

	expiresAt := calculateExpiry(payload, time.Now())

	return payload.AccessToken, expiresAt, nil
}

type refreshResponse struct {
	AccessToken  string `json:"AccessToken"`
	RefreshToken string `json:"RefreshToken"`
	ExpiresIn    int    `json:"ExpiresIn"`
	Expire       string `json:"Expire"`
}

func calculateExpiry(payload refreshResponse, now time.Time) time.Time {
	if payload.ExpiresIn > 0 {
		return now.Add(time.Duration(payload.ExpiresIn) * time.Second)
	}

	if payload.Expire != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, payload.Expire); err == nil {
			return parsed
		}
		if parsed, err := time.Parse(time.RFC3339, payload.Expire); err == nil {
			return parsed
		}
	}

	return now.Add(defaultTokenLifetime)
}
