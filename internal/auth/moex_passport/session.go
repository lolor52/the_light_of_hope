package moexpassport

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

const (
	authEndpointURL    = "https://passport.moex.com/authenticate"
	langQueryParam     = "lang"
	langValue          = "ru"
	passportCookie     = "MicexPassportCert"
	issBaseURL         = "https://iss.moex.com/"
	defaultHTTPTimeout = 15 * time.Second
)

// Session представляет авторизованную HTTP-сессию MOEX Passport.
type Session struct {
	httpClient *http.Client
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
		log.Printf("passport auth request failed: %v", err)
		return nil, fmt.Errorf("passport auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("passport auth unexpected status: %s", resp.Status)
		return nil, fmt.Errorf("passport auth unexpected status: %s", resp.Status)
	}

	issURL, err := url.Parse(issBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse iss url: %w", err)
	}

	var cookieFound bool
	for _, cookie := range jar.Cookies(issURL) {
		if cookie.Name == passportCookie {
			cookieFound = true
			break
		}
	}
	if !cookieFound {
		log.Print("passport auth cookie MicexPassportCert not received")
		return nil, errors.New("passport auth cookie MicexPassportCert not received")
	}

	return &Session{
		httpClient: httpClient,
	}, nil
}

// HTTPClient возвращает авторизованный HTTP-клиент.
func (s *Session) HTTPClient() *http.Client {
	return s.httpClient
}
