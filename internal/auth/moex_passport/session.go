package moexpassport

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
	authEndpointURL    = "https://passport.moex.com/authenticate"
	langQueryParam     = "lang"
	langValue          = "ru"
	passportCookie     = "MicexPassportCert"
	markerHeader       = "X-MicexPassport-Marker"
	grantedMarker      = "granted"
	defaultHTTPTimeout = 15 * time.Second
)

// Session представляет авторизованную HTTP-сессию MOEX Passport.
type Session struct {
	httpClient *http.Client
	marker     string
}

// Authenticate выполняет авторизацию в MOEX Passport и возвращает сессию
// с установленным cookie MicexPassportCert.
func Authenticate(ctx context.Context, login, password string) (*Session, error) {
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
		Timeout:   defaultHTTPTimeout,
		Transport: http.DefaultTransport,
		Jar:       jar,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authEndpointURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create auth request: %w", err)
	}
	req.SetBasicAuth(login, password)

	query := req.URL.Query()
	query.Set(langQueryParam, langValue)
	req.URL.RawQuery = query.Encode()

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("passport auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("passport auth unexpected status: %s", resp.Status)
	}

	marker := resp.Header.Get(markerHeader)
	if marker == "" {
		return nil, errors.New("passport auth marker header is missing")
	}
	if !strings.EqualFold(marker, grantedMarker) {
		return nil, fmt.Errorf("passport auth marker is not granted: %s", marker)
	}

	var payload struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode passport auth response: %w", err)
	}
	if !payload.Success {
		if payload.Error == "" {
			payload.Error = "unknown passport error"
		}
		return nil, errors.New(payload.Error)
	}

	requestURL := resp.Request.URL
	if requestURL == nil {
		parsed, parseErr := url.Parse(authEndpointURL)
		if parseErr != nil {
			return nil, fmt.Errorf("determine auth url: %w", parseErr)
		}
		requestURL = parsed
	}

	var cookieFound bool
	for _, cookie := range jar.Cookies(requestURL) {
		if cookie.Name == passportCookie {
			cookieFound = true
			break
		}
	}
	if !cookieFound {
		return nil, errors.New("passport auth cookie MicexPassportCert not received")
	}

	return &Session{
		httpClient: httpClient,
		marker:     marker,
	}, nil
}

// HTTPClient возвращает авторизованный HTTP-клиент.
func (s *Session) HTTPClient() *http.Client {
	return s.httpClient
}

// Marker возвращает значение заголовка X-MicexPassport-Marker, полученное при авторизации.
func (s *Session) Marker() string {
	return s.marker
}
