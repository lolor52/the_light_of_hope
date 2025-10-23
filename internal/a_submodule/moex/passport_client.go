package moex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const passportTokenURL = "https://passport.moex.com/token"

// Token описывает ответ сервиса MOEX Passport.
type Token struct {
	AccessToken      string        `json:"access_token"`
	TokenType        string        `json:"token_type"`
	ExpiresIn        time.Duration `json:"-"`
	RefreshToken     string        `json:"refresh_token"`
	ExpiresInSeconds int64         `json:"expires_in"`
}

// expireDuration возвращает длительность действия токена.
func (t *Token) expireDuration() time.Duration {
	if t.ExpiresInSeconds <= 0 {
		return 0
	}

	t.ExpiresIn = time.Duration(t.ExpiresInSeconds) * time.Second
	return t.ExpiresIn
}

// PassportClient отвечает за получение токена авторизации MOEX Passport.
type PassportClient struct {
	login      string
	password   string
	httpClient *http.Client
}

// NewPassportClient создаёт клиент авторизации MOEX Passport.
func NewPassportClient(login, password string, httpClient *http.Client) (*PassportClient, error) {
	if strings.TrimSpace(login) == "" {
		return nil, errors.New("login is empty")
	}
	if strings.TrimSpace(password) == "" {
		return nil, errors.New("password is empty")
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &PassportClient{
		login:      login,
		password:   password,
		httpClient: httpClient,
	}, nil
}

// Token получает access token для последующих запросов к MOEX ISS.
func (c *PassportClient) Token(ctx context.Context) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", c.login)
	form.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, passportTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, fmt.Errorf("create passport request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("do passport request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Token{}, fmt.Errorf("read passport response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Token{}, fmt.Errorf("passport status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return Token{}, fmt.Errorf("decode passport response: %w", err)
	}

	token.expireDuration()

	if token.AccessToken == "" {
		return Token{}, errors.New("passport token is empty")
	}

	return token, nil
}
