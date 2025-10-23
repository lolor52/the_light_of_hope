package indicators

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCalcVWAP(t *testing.T) {
	trades := []trade{
		{Price: 100, Volume: 10},
		{Price: 110, Volume: 20},
		{Price: 90, Volume: 5},
	}

	vwap, err := calcVWAP(trades)
	if err != nil {
		t.Fatalf("calcVWAP returned error: %v", err)
	}

	numerator := 100.0*10 + 110.0*20 + 90.0*5
	denominator := 10.0 + 20.0 + 5.0
	want := numerator / denominator
	if math.Abs(vwap-want) > 1e-9 {
		t.Fatalf("unexpected VWAP: got %f, want %f", vwap, want)
	}
}

func TestCalcValueArea(t *testing.T) {
	trades := []trade{
		{Price: 99.9, Volume: 10},
		{Price: 100, Volume: 80},
		{Price: 100.1, Volume: 15},
		{Price: 100.2, Volume: 5},
	}

	val, vah, err := calcValueArea(trades)
	if err != nil {
		t.Fatalf("calcValueArea returned error: %v", err)
	}

	if val > 100 || val < 99.9 {
		t.Fatalf("unexpected VAL: %f", val)
	}
	if vah < 100 || vah > 100.1 {
		t.Fatalf("unexpected VAH: %f", vah)
	}
}

func TestMainSessionBounds(t *testing.T) {
	loc, err := time.LoadLocation(moscowLocationName)
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	date := time.Date(2024, 5, 20, 15, 30, 0, 0, time.UTC)
	start, end, err := mainSessionBounds(date)
	if err != nil {
		t.Fatalf("mainSessionBounds error: %v", err)
	}

	expectedStart := time.Date(2024, 5, 20, mainSessionStartHour, 0, 0, 0, loc)
	expectedEnd := time.Date(2024, 5, 20, mainSessionEndHour, mainSessionEndMinute, 0, 0, loc)

	if !start.Equal(expectedStart) {
		t.Fatalf("unexpected start: got %v, want %v", start, expectedStart)
	}
	if !end.Equal(expectedEnd) {
		t.Fatalf("unexpected end: got %v, want %v", end, expectedEnd)
	}
}

func TestFetchTradesReturnsInvalidRequestOn404(t *testing.T) {
	token := "token"
	from := time.Date(2024, 5, 20, 7, 0, 0, 0, time.UTC)
	to := time.Date(2024, 5, 20, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("token"); got != token {
			t.Fatalf("unexpected token query: %q", got)
		}
		if got := r.URL.Query().Get("instrumentGroup"); got != "TQBR" {
			t.Fatalf("unexpected instrumentGroup: %q", got)
		}
		if got := r.URL.Query().Get("from"); got != strconv.FormatInt(from.UTC().Unix(), 10) {
			t.Fatalf("unexpected from query: %q", got)
		}
		if got := r.URL.Query().Get("to"); got != strconv.FormatInt(to.UTC().Unix(), 10) {
			t.Fatalf("unexpected to query: %q", got)
		}
		if got := r.URL.Path; got != "/md/v2/Securities/MOEX/BAD/alltrades" {
			t.Fatalf("unexpected path: %q", got)
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	provider := staticTokenProvider{token: token}
	client := NewMarketDataClient(server.URL, provider)
	client.WithHTTPClient(server.Client())

	_, err := client.FetchTrades(context.Background(), "TQBR", "BAD", from, to)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}

	response := client.LastResponse()
	if !strings.Contains(response, "status=404") {
		t.Fatalf("expected response snapshot with status, got %q", response)
	}
}

type staticTokenProvider struct {
	token string
}

func (s staticTokenProvider) AccessToken(context.Context) (string, error) {
	return s.token, nil
}
