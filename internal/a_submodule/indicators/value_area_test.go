package indicators

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"invest_intraday/internal/a_submodule/alor"
	"invest_intraday/models"
)

type fakeTickerInfoRepo struct {
	info   models.TickerInfo
	err    error
	calls  int
	lastID int64
}

func (f *fakeTickerInfoRepo) GetByID(ctx context.Context, id int64) (models.TickerInfo, error) {
	f.calls++
	f.lastID = id
	if f.err != nil {
		return models.TickerInfo{}, f.err
	}
	return f.info, nil
}

type fakeTradeProvider struct {
	trades         []alor.Trade
	err            error
	calls          int
	lastInstrument alor.Instrument
	lastDate       time.Time
}

func (f *fakeTradeProvider) Trades(ctx context.Context, instrument alor.Instrument, sessionDate time.Time) ([]alor.Trade, error) {
	f.calls++
	f.lastInstrument = instrument
	f.lastDate = sessionDate
	if f.err != nil {
		return nil, f.err
	}
	// возвращаем копию, чтобы симулировать поведение клиента ALOR
	copied := make([]alor.Trade, len(f.trades))
	copy(copied, f.trades)
	return copied, nil
}

func TestCalculatorCalculateUsesOnlyAlorTrades(t *testing.T) {
	repo := &fakeTickerInfoRepo{info: models.TickerInfo{
		ID:         12,
		TickerName: "SBER",
		SecID:      "sber",
		BoardID:    "TQBR",
	}}
	provider := &fakeTradeProvider{trades: []alor.Trade{
		{Price: 100, Quantity: 1, TradingSession: "DAY", TradeTime: "10:00:05"},
		{Price: 200, Quantity: 1, TradingSession: "DAY", TradeTime: "10:01:15"},
	}}

	calc := NewCalculator(repo, provider)
	sessionDate := time.Date(2024, time.January, 15, 0, 0, 0, 0, time.UTC)

	result, err := calc.Calculate(context.Background(), 12, sessionDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.calls != 1 {
		t.Fatalf("expected repository to be called once, got %d", repo.calls)
	}
	if repo.lastID != 12 {
		t.Fatalf("unexpected ticker id: %d", repo.lastID)
	}

	if provider.calls != 1 {
		t.Fatalf("expected ALOR provider to be called once, got %d", provider.calls)
	}
	if provider.lastInstrument.Exchange != alor.ExchangeMOEX || provider.lastInstrument.Board != "TQBR" || provider.lastInstrument.Symbol != "SBER" {
		t.Fatalf("unexpected instrument passed to provider: %+v", provider.lastInstrument)
	}
	if !provider.lastDate.Equal(sessionDate) {
		t.Fatalf("unexpected session date: %s", provider.lastDate)
	}

	if math.Abs(result.VWAP-150) > 1e-9 {
		t.Fatalf("unexpected VWAP: %f", result.VWAP)
	}
	if math.Abs(result.VAL-100) > 1e-9 {
		t.Fatalf("unexpected VAL: %f", result.VAL)
	}
	if math.Abs(result.VAH-200) > 1e-9 {
		t.Fatalf("unexpected VAH: %f", result.VAH)
	}
}

func TestCalculatorReturnsErrorWhenNoTrades(t *testing.T) {
	repo := &fakeTickerInfoRepo{info: models.TickerInfo{ID: 7, TickerName: "GAZP", BoardID: "TQBR"}}
	provider := &fakeTradeProvider{trades: nil}

	calc := NewCalculator(repo, provider)
	_, err := calc.Calculate(context.Background(), 7, time.Date(2024, time.January, 10, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrNoTrades) {
		t.Fatalf("expected ErrNoTrades, got %v", err)
	}
}
