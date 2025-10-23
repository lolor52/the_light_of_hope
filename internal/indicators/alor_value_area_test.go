package indicators

import (
	"math"
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
