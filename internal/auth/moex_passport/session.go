package moexpassport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

const (
	authEndpoint       = "https://passport.moex.com/authenticate"
	issBaseURL         = "https://iss.moex.com"
	passportCookieName = "MicexPassportCert"
	markerHeader       = "X-MicexPassport-Marker"
	markerGranted      = "granted"
)

// Session представляет авторизованную сессию MOEX Passport.
type Session struct {
	httpClient *http.Client
}

// NewSession выполняет авторизацию через MOEX Passport и возвращает готовую HTTP-сессию.
func NewSession(ctx context.Context, login, password string) (*Session, error) {
	if login == "" {
		return nil, errors.New("login is required")
	}
	if password == "" {
		return nil, errors.New("password is required")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}

	httpClient := &http.Client{
		Timeout:   15 * time.Second,
		Transport: http.DefaultTransport,
		Jar:       jar,
	}

	if err := authenticate(ctx, httpClient, login, password); err != nil {
		return nil, err
	}

	return &Session{httpClient: httpClient}, nil
}

// HTTPClient возвращает HTTP-клиент с сохранёнными cookie Passport.
func (s *Session) HTTPClient() *http.Client {
	return s.httpClient
}

func authenticate(ctx context.Context, httpClient *http.Client, login, password string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authEndpoint, nil)
	if err != nil {
		return fmt.Errorf("create auth request: %w", err)
	}

	req.SetBasicAuth(login, password)
	req.Header.Set("Accept", "application/json")

	query := req.URL.Query()
	query.Set("lang", "ru")
	req.URL.RawQuery = query.Encode()

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("passport auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("passport auth unexpected status: %s", resp.Status)
	}

	marker := resp.Header.Get(markerHeader)
	if marker != markerGranted {
		if marker == "" {
			return errors.New("passport access marker not granted")
		}
		return fmt.Errorf("passport access marker: %s", marker)
	}

	var payload struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode passport auth response: %w", err)
	}
	if !payload.Success {
		if payload.Error == "" {
			payload.Error = "passport authentication failed"
		}
		return errors.New(payload.Error)
	}

	authURL, err := url.Parse(authEndpoint)
	if err != nil {
		return fmt.Errorf("parse auth endpoint: %w", err)
	}

	if !hasPassportCookie(httpClient.Jar, authURL) {
		issURL, parseErr := url.Parse(issBaseURL)
		if parseErr != nil {
			return fmt.Errorf("parse iss endpoint: %w", parseErr)
		}
		if !hasPassportCookie(httpClient.Jar, issURL) {
			return errors.New("passport auth cookie not received")
		}
	}

	return nil
}

func hasPassportCookie(jar http.CookieJar, u *url.URL) bool {
	if jar == nil {
		return false
	}
	for _, cookie := range jar.Cookies(u) {
		if cookie.Name == passportCookieName && cookie.Value != "" {
			return true
		}
	}
	return false
}
