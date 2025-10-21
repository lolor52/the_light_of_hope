package tickers_selection

import (
	"math"
	"testing"
)

func TestCalculateScoresRange(t *testing.T) {
	sessions := []sessionMetrics{
		{VAH: 110, VAL: 90, VWAP: 100, Liquidity: 10, Volatility: 60, FlatTrend: 20},
		{VAH: 111, VAL: 91, VWAP: 100, Liquidity: 20, Volatility: 60, FlatTrend: 20},
		{VAH: 112, VAL: 92, VWAP: 100, Liquidity: 30, Volatility: 60, FlatTrend: 20},
		{VAH: 110, VAL: 90, VWAP: 100, Liquidity: 40, Volatility: 60, FlatTrend: 20},
		{VAH: 108, VAL: 92, VWAP: 100, Liquidity: 50, Volatility: 60, FlatTrend: 20},
	}

	scores, err := calculateScores(sessions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if scores.Regime != RegimeRange {
		t.Fatalf("expected RegimeRange, got %s", scores.Regime)
	}

	expectedFinal := 37.85
	if math.Abs(scores.FinalScore-expectedFinal) > 1e-2 {
		t.Fatalf("unexpected final score: %.2f", scores.FinalScore)
	}

	if scores.FinalScore != scores.MeanReversionScore {
		t.Fatalf("final score should equal mean reversion score in range regime")
	}

	if scores.Breakout {
		t.Fatalf("breakout should be false")
	}
}

func TestCalculateScoresTrend(t *testing.T) {
	sessions := []sessionMetrics{
		{VAH: 95, VAL: 85, VWAP: 90, Liquidity: 70, Volatility: 60, FlatTrend: 40},
		{VAH: 100, VAL: 90, VWAP: 95, Liquidity: 75, Volatility: 65, FlatTrend: 35},
		{VAH: 105, VAL: 95, VWAP: 100, Liquidity: 80, Volatility: 70, FlatTrend: 30},
		{VAH: 110, VAL: 100, VWAP: 100, Liquidity: 85, Volatility: 75, FlatTrend: 20},
		{VAH: 120, VAL: 110, VWAP: 121, Liquidity: 90, Volatility: 80, FlatTrend: 10},
	}

	scores, err := calculateScores(sessions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if scores.Regime != RegimeTrend {
		t.Fatalf("expected RegimeTrend, got %s", scores.Regime)
	}

	expectedFinal := 73.68
	if math.Abs(scores.FinalScore-expectedFinal) > 1e-2 {
		t.Fatalf("unexpected final score: %.2f", scores.FinalScore)
	}

	if scores.FinalScore != scores.MomentumScore {
		t.Fatalf("final score should equal momentum score in trend regime")
	}

	if !scores.Breakout {
		t.Fatalf("breakout should be true")
	}
}

func TestClamp(t *testing.T) {
	if clamp(-1, 0, 10) != 0 {
		t.Fatalf("clamp should return min for lower input")
	}
	if clamp(11, 0, 10) != 10 {
		t.Fatalf("clamp should return max for higher input")
	}
	if clamp(5, 0, 10) != 5 {
		t.Fatalf("clamp should pass through value in range")
	}
}
