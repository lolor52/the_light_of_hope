package indicators

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
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

type stubTokenProvider struct {
	token string
	err   error
}

func (s stubTokenProvider) AccessToken(_ context.Context) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.token, nil
}

func TestMarketDataClientFetchTradesInvalidInstrument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := NewMarketDataClient(server.URL, stubTokenProvider{token: "token"})
	client.WithHTTPClient(server.Client())

	_, err := client.FetchTrades(context.Background(), "TQBR", "BAD", time.Now().Add(-time.Hour), time.Now())
	if !errors.Is(err, ErrInvalidInstrument) {
		t.Fatalf("expected ErrInvalidInstrument, got %v", err)
	}

	if got := client.LastResponse(); got != "status 404 without body" {
		t.Fatalf("unexpected last response: %q", got)
	}
}

func TestMarketDataClientFetchTradesNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := NewMarketDataClient(server.URL, stubTokenProvider{token: "token"})
	client.WithHTTPClient(server.Client())

	_, err := client.FetchTrades(context.Background(), "TQBR", "EMPTY", time.Now().Add(-time.Hour), time.Now())
	if !errors.Is(err, ErrNoTrades) {
		t.Fatalf("expected ErrNoTrades, got %v", err)
	}

	if got := client.LastResponse(); got != "status 204 without body" {
		t.Fatalf("unexpected last response: %q", got)
	}
}
