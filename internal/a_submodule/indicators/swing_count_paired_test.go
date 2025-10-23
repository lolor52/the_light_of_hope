package indicators

import (
	"context"
	"testing"
	"time"

	"invest_intraday/internal/a_submodule/alor"
	"invest_intraday/models"
)

func TestSwingCountPairedCalculatorUsesAlorTrades(t *testing.T) {
	repo := &fakeTickerInfoRepo{info: models.TickerInfo{
		ID:         5,
		TickerName: "GMKN",
		SecID:      "gmkn",
		BoardID:    "TQBR",
	}}
	provider := &fakeTradeProvider{trades: []alor.Trade{
		{Price: 1500, Quantity: 1, TradingSession: "DAY", TradeTime: "10:00:05"},
		{Price: 1501, Quantity: 1, TradingSession: "DAY", TradeTime: "10:01:05"},
		{Price: 1502, Quantity: 1, TradingSession: "DAY", TradeTime: "10:02:05"},
	}}

	calc := NewSwingCountPairedCalculator(repo, provider, DefaultSwingCountParams())
	sessionDate := time.Date(2024, time.January, 17, 0, 0, 0, 0, time.UTC)

	value, err := calc.Calculate(context.Background(), 5, sessionDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.calls != 1 {
		t.Fatalf("expected repository to be called once, got %d", repo.calls)
	}
	if provider.calls != 1 {
		t.Fatalf("expected ALOR provider to be called once, got %d", provider.calls)
	}
	if provider.lastInstrument.Exchange != alor.ExchangeMOEX || provider.lastInstrument.Board != "TQBR" || provider.lastInstrument.Symbol != "GMKN" {
		t.Fatalf("unexpected instrument passed to provider: %+v", provider.lastInstrument)
	}

	if value != 0 {
		t.Fatalf("expected zero swing pairs for monotonic data, got %d", value)
	}
}

func TestSwingCountPairedCalculatorNoTrades(t *testing.T) {
	repo := &fakeTickerInfoRepo{info: models.TickerInfo{ID: 9, TickerName: "LKOH", BoardID: "TQBR"}}
	provider := &fakeTradeProvider{trades: nil}

	calc := NewSwingCountPairedCalculator(repo, provider, DefaultSwingCountParams())
	_, err := calc.Calculate(context.Background(), 9, time.Date(2024, time.January, 5, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatalf("expected error due to missing trades")
	}
	if provider.calls != 1 {
		t.Fatalf("expected ALOR provider to be called once, got %d", provider.calls)
	}
}
