package alor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseJWTExpiration(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	token := buildTestJWT(now.Add(5 * time.Minute))

	got, err := parseJWTExpiration(token)
	if err != nil {
		t.Fatalf("parseJWTExpiration returned error: %v", err)
	}

	if !got.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("unexpected expiration: %v", got)
	}

	if _, err := parseJWTExpiration("invalid.token"); err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestClientCheckAuthorizationUsesCachedToken(t *testing.T) {
	tokenValue := buildTestJWT(time.Now().Add(10 * time.Minute))

	apiCalls := int32(0)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		if got := r.Header.Get("Authorization"); got != "Bearer "+tokenValue {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		if got := r.URL.Query().Get("token"); got != tokenValue {
			t.Fatalf("unexpected token query: %q", got)
		}

		_, _ = w.Write([]byte(`{"levels":[]}`))
	}))
	t.Cleanup(api.Close)

	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("refresh endpoint must not be called")
	}))
	t.Cleanup(oauth.Close)

	client, err := NewClient("refresh-token", WithEnvironment(Environment{OAuthURL: oauth.URL, APIURL: api.URL}))
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	client.token = &accessToken{value: tokenValue, expiresAt: time.Now().Add(10 * time.Minute)}

	if err := client.CheckAuthorization(context.Background()); err != nil {
		t.Fatalf("CheckAuthorization returned error: %v", err)
	}

	if atomic.LoadInt32(&apiCalls) != 1 {
		t.Fatalf("expected 1 API call, got %d", apiCalls)
	}
}

func TestClientCheckAuthorizationRefreshesWhenTokenAboutToExpire(t *testing.T) {
	newToken := buildTestJWT(time.Now().Add(10 * time.Minute))

	var refreshCalls int32
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&refreshCalls, 1)
		_, _ = fmt.Fprintf(w, `{"AccessToken":"%s"}`, newToken)
	}))
	t.Cleanup(oauth.Close)

	apiCalls := int32(0)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		if got := r.Header.Get("Authorization"); got != "Bearer "+newToken {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		if got := r.URL.Query().Get("token"); got != newToken {
			t.Fatalf("unexpected token query: %q", got)
		}

		_, _ = w.Write([]byte(`{"levels":[]}`))
	}))
	t.Cleanup(api.Close)

	client, err := NewClient("refresh-token", WithEnvironment(Environment{OAuthURL: oauth.URL, APIURL: api.URL}))
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	client.refreshBefore = time.Minute
	client.token = &accessToken{value: buildTestJWT(time.Now().Add(30 * time.Second)), expiresAt: time.Now().Add(30 * time.Second)}

	if err := client.CheckAuthorization(context.Background()); err != nil {
		t.Fatalf("CheckAuthorization returned error: %v", err)
	}

	if atomic.LoadInt32(&refreshCalls) != 1 {
		t.Fatalf("expected 1 refresh call, got %d", refreshCalls)
	}

	if atomic.LoadInt32(&apiCalls) != 1 {
		t.Fatalf("expected 1 API call, got %d", apiCalls)
	}
}

func TestClientCheckAuthorizationRetriesOnUnauthorized(t *testing.T) {
	oldToken := buildTestJWT(time.Now().Add(10 * time.Minute))
	newToken := buildTestJWT(time.Now().Add(20 * time.Minute))

	var refreshCalls int32
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&refreshCalls, 1)
		_, _ = fmt.Fprintf(w, `{"AccessToken":"%s"}`, newToken)
	}))
	t.Cleanup(oauth.Close)

	var seenNewToken atomic.Bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		tokenQuery := r.URL.Query().Get("token")
		switch auth {
		case "Bearer " + oldToken:
			if tokenQuery != oldToken {
				t.Fatalf("unexpected token query for old token: %q", tokenQuery)
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		case "Bearer " + newToken:
			if tokenQuery != newToken {
				t.Fatalf("unexpected token query for new token: %q", tokenQuery)
			}
			seenNewToken.Store(true)
			_, _ = w.Write([]byte(`{"levels":[]}`))
			return
		default:
			t.Fatalf("unexpected Authorization header: %q", auth)
		}
	}))
	t.Cleanup(api.Close)

	client, err := NewClient("refresh-token", WithEnvironment(Environment{OAuthURL: oauth.URL, APIURL: api.URL}))
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	client.token = &accessToken{value: oldToken, expiresAt: time.Now().Add(10 * time.Minute)}

	if err := client.CheckAuthorization(context.Background()); err != nil {
		t.Fatalf("CheckAuthorization returned error: %v", err)
	}

	if !seenNewToken.Load() {
		t.Fatal("expected new token to be used on retry")
	}

	if atomic.LoadInt32(&refreshCalls) != 1 {
		t.Fatalf("expected single refresh call, got %d", refreshCalls)
	}
}

func TestClientCheckAuthorizationUnauthorizedAfterRefresh(t *testing.T) {
	oldToken := buildTestJWT(time.Now().Add(10 * time.Minute))
	newToken := buildTestJWT(time.Now().Add(20 * time.Minute))

	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"AccessToken":"%s"}`, newToken)
	}))
	t.Cleanup(oauth.Close)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("token"); got == "" {
			t.Fatal("expected token query parameter")
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(api.Close)

	client, err := NewClient("refresh-token", WithEnvironment(Environment{OAuthURL: oauth.URL, APIURL: api.URL}))
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	client.token = &accessToken{value: oldToken, expiresAt: time.Now().Add(10 * time.Minute)}

	err = client.CheckAuthorization(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestClientCheckAuthorizationUnexpectedStatus(t *testing.T) {
	token := buildTestJWT(time.Now().Add(10 * time.Minute))

	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"AccessToken":"%s"}`, token)
	}))
	t.Cleanup(oauth.Close)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("token"); got == "" {
			t.Fatal("expected token query parameter")
		}
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	t.Cleanup(api.Close)

	client, err := NewClient("refresh-token", WithEnvironment(Environment{OAuthURL: oauth.URL, APIURL: api.URL}))
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	if err := client.CheckAuthorization(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func buildTestJWT(expiresAt time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadMap := map[string]int64{"exp": expiresAt.Unix()}
	payloadBytes, _ := json.Marshal(payloadMap)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return strings.Join([]string{header, payload, "signature"}, ".")
}
