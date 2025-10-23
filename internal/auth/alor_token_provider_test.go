package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAlorTokenProviderReturnsToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "refresh" {
			t.Fatalf("unexpected refresh token %q", r.URL.Query().Get("token"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"AccessToken": "access-1",
			"ExpiresIn":   1800,
		})
	}))
	defer server.Close()

	provider, err := NewAlorTokenProvider("refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	provider.endpoint = server.URL

	token, err := provider.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken returned error: %v", err)
	}
	if token != "access-1" {
		t.Fatalf("unexpected token %q", token)
	}
}

func TestAlorTokenProviderCachesToken(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"AccessToken": "access-2",
			"ExpiresIn":   1800,
		})
	}))
	defer server.Close()

	provider, err := NewAlorTokenProvider("refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	provider.endpoint = server.URL

	ctx := context.Background()
	if _, err := provider.AccessToken(ctx); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if _, err := provider.AccessToken(ctx); err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected single HTTP call, got %d", got)
	}
}

func TestAlorTokenProviderRefreshesExpiredToken(t *testing.T) {
	var tokenIndex int32
	tokens := []string{"access-old", "access-new"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := atomic.LoadInt32(&tokenIndex)
		json.NewEncoder(w).Encode(map[string]any{
			"AccessToken": tokens[idx],
			"ExpiresIn":   1,
		})
		atomic.AddInt32(&tokenIndex, 1)
	}))
	defer server.Close()

	provider, err := NewAlorTokenProvider("refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	provider.endpoint = server.URL

	ctx := context.Background()

	first, err := provider.AccessToken(ctx)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if first != "access-old" {
		t.Fatalf("unexpected token %q", first)
	}

	time.Sleep(1500 * time.Millisecond)

	second, err := provider.AccessToken(ctx)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if second != "access-new" {
		t.Fatalf("expected refreshed token, got %q", second)
	}
}

func TestAlorTokenProviderInvalidateForcesRefresh(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"AccessToken": fmt.Sprintf("access-%d", call),
			"ExpiresIn":   1800,
		})
	}))
	defer server.Close()

	provider, err := NewAlorTokenProvider("refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	provider.endpoint = server.URL

	ctx := context.Background()
	first, err := provider.AccessToken(ctx)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	provider.Invalidate()

	second, err := provider.AccessToken(ctx)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if first == second {
		t.Fatalf("expected different tokens after invalidation")
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected two HTTP calls, got %d", calls)
	}
}

func TestAlorTokenProviderFailsOnEmptyAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"AccessToken": "",
		})
	}))
	defer server.Close()

	provider, err := NewAlorTokenProvider("refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	provider.endpoint = server.URL

	if _, err := provider.AccessToken(context.Background()); err == nil {
		t.Fatalf("expected error for empty access token")
	}
}
