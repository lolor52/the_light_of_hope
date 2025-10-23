package alor

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientCheckAuthorizationSuccessAndCaching(t *testing.T) {
	t.Parallel()

	expectedExpiry := time.Now().Add(10 * time.Minute).UTC()
	accessToken := makeJWT(expectedExpiry)

	var oauthCalls int32
	oauthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&oauthCalls, 1)
		if r.Method != http.MethodPost {
			t.Fatalf("ожидался POST, получен %s", r.Method)
		}
		_ = r.ParseForm()
		if r.FormValue("token") == "" {
			t.Fatal("token передан пустым")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"AccessToken":"%s"}`, accessToken)
	}))
	defer oauthSrv.Close()

	var apiCalls int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		want := "Bearer " + accessToken
		if got := r.Header.Get("Authorization"); got != want {
			t.Fatalf("неверный Authorization: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"bids":[],"asks":[]}`))
	}))
	defer apiSrv.Close()

	client, err := NewClient("refresh", WithBaseURLs(oauthSrv.URL, apiSrv.URL), WithHTTPClient(&http.Client{Timeout: time.Second}))
	if err != nil {
		t.Fatalf("не удалось создать клиент: %v", err)
	}
	// Используем http.Client с тайм-аутом, так как тестовые серверы работают по HTTP.

	status, err := client.CheckAuthorization(context.Background())
	if err != nil {
		t.Fatalf("CheckAuthorization вернул ошибку: %v", err)
	}
	if !status.Authorized {
		t.Fatalf("ожидалась авторизация, получено %#v", status)
	}
	if diff := status.ExpiresAt.Sub(expectedExpiry); diff > time.Second || diff < -time.Second {
		t.Fatalf("неожиданное время истечения токена: %v", status.ExpiresAt)
	}

	if calls := atomic.LoadInt32(&oauthCalls); calls != 1 {
		t.Fatalf("ожидался один вызов OAuth, получено %d", calls)
	}

	status, err = client.CheckAuthorization(context.Background())
	if err != nil {
		t.Fatalf("CheckAuthorization (2) вернул ошибку: %v", err)
	}
	if !status.Authorized {
		t.Fatalf("повторный вызов должен быть авторизован")
	}

	if calls := atomic.LoadInt32(&oauthCalls); calls != 1 {
		t.Fatalf("ожидалось кэширование access-токена, вызовов OAuth: %d", calls)
	}
	if calls := atomic.LoadInt32(&apiCalls); calls != 2 {
		t.Fatalf("ожидалось два обращения к приватному API, получено %d", calls)
	}
}

func TestClientCheckAuthorizationRefreshOnUnauthorized(t *testing.T) {
	t.Parallel()

	token1 := makeJWT(time.Now().Add(5 * time.Minute))
	token2 := makeJWT(time.Now().Add(10 * time.Minute))

	var oauthCalls int32
	oauthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&oauthCalls, 1)

		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			fmt.Fprintf(w, `{"AccessToken":"%s","RefreshToken":"refresh-2"}`, token1)
			return
		}
		fmt.Fprintf(w, `{"AccessToken":"%s"}`, token2)
	}))
	defer oauthSrv.Close()

	var apiCalls int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&apiCalls, 1)
		if call == 1 {
			want := "Bearer " + token1
			if got := r.Header.Get("Authorization"); got != want {
				t.Fatalf("неверный Authorization на первом вызове: %q", got)
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		want := "Bearer " + token2
		if got := r.Header.Get("Authorization"); got != want {
			t.Fatalf("неверный Authorization на повторном вызове: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer apiSrv.Close()

	client, err := NewClient("refresh-1", WithBaseURLs(oauthSrv.URL, apiSrv.URL), WithHTTPClient(&http.Client{Timeout: time.Second}))
	if err != nil {
		t.Fatalf("не удалось создать клиент: %v", err)
	}

	status, err := client.CheckAuthorization(context.Background())
	if err != nil {
		t.Fatalf("CheckAuthorization вернул ошибку: %v", err)
	}
	if !status.Authorized {
		t.Fatalf("ожидалась успешная авторизация после обновления токена")
	}

	if calls := atomic.LoadInt32(&oauthCalls); calls != 2 {
		t.Fatalf("ожидалось два обращения к OAuth, получено %d", calls)
	}
	if calls := atomic.LoadInt32(&apiCalls); calls != 2 {
		t.Fatalf("ожидалось два обращения к приватному API, получено %d", calls)
	}
}

func TestClientCheckAuthorizationInvalidJSON(t *testing.T) {
	t.Parallel()

	accessToken := makeJWT(time.Now().Add(5 * time.Minute))

	oauthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"AccessToken":"%s"}`, accessToken)
	}))
	defer oauthSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json"))
	}))
	defer apiSrv.Close()

	client, err := NewClient("refresh", WithBaseURLs(oauthSrv.URL, apiSrv.URL), WithHTTPClient(&http.Client{Timeout: time.Second}))
	if err != nil {
		t.Fatalf("не удалось создать клиент: %v", err)
	}

	if _, err := client.CheckAuthorization(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка при некорректном JSON")
	}
}

func makeJWT(exp time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix())))
	return header + "." + payload + "."
}
