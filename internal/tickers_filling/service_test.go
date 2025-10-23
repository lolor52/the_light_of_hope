package tickers_filling

import (
	"context"
	"errors"
	"testing"
	"time"

	"invest_intraday/internal/a_technical/db"
	"invest_intraday/internal/indicators"
	"invest_intraday/models"
)

func TestService_Fill_InsertsMissingSessions(t *testing.T) {
	ctx := context.Background()

	ticker := models.TickerInfo{ID: 1, TickerName: "GAZP", BoardID: boardTQBR}
	existingDate := time.Date(2025, 10, 20, 0, 0, 0, 0, time.UTC)

	infoRepo := &fakeTickerInfoRepo{items: []models.TickerInfo{ticker}}
	historyRepo := &fakeTickerHistoryRepo{
		records: map[string]map[string]models.TickerHistory{
			"GAZP": {
				existingDate.Format(time.DateOnly): {
					TradingSessionActive: true,
				},
			},
		},
	}

	calc := &fakeCalculator{
		profiles: map[string]indicators.SessionProfile{
			"2025-10-19": {VWAP: 101.123456, VAL: 99.1, VAH: 103.9},
		},
	}

	now := time.Date(2025, 10, 21, 11, 0, 0, 0, time.UTC)

	svc, err := NewService(infoRepo, historyRepo, calc, 2, 5, WithNowFunc(func() time.Time { return now }), WithLocation(time.UTC))
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	if err := svc.Fill(ctx); err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	if len(historyRepo.inserted) != 1 {
		t.Fatalf("expected 1 inserted record, got %d", len(historyRepo.inserted))
	}

	record := historyRepo.inserted[0]
	if !record.TradingSessionActive {
		t.Fatalf("expected TradingSessionActive true")
	}
	if record.VWAP == nil || *record.VWAP != "101.123456" {
		t.Fatalf("unexpected VWAP: %v", record.VWAP)
	}
	if record.VAL == nil || *record.VAL != "99.1" {
		t.Fatalf("unexpected VAL: %v", record.VAL)
	}
	if record.VAH == nil || *record.VAH != "103.9" {
		t.Fatalf("unexpected VAH: %v", record.VAH)
	}
}

func TestService_Fill_InsertsInactiveSession(t *testing.T) {
	ctx := context.Background()

	ticker := models.TickerInfo{ID: 7, TickerName: "ROSN", BoardID: boardTQBR}

	infoRepo := &fakeTickerInfoRepo{items: []models.TickerInfo{ticker}}
	historyRepo := &fakeTickerHistoryRepo{}
	calc := &fakeCalculator{err: indicators.ErrNoTrades}

	now := time.Date(2025, 10, 21, 10, 0, 0, 0, time.UTC)

	svc, err := NewService(infoRepo, historyRepo, calc, 1, 3, WithNowFunc(func() time.Time { return now }), WithLocation(time.UTC))
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	if err := svc.Fill(ctx); err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	if len(historyRepo.inserted) != 3 {
		t.Fatalf("expected 3 inserted records (maxInactive days), got %d", len(historyRepo.inserted))
	}

	for _, record := range historyRepo.inserted {
		if record.TradingSessionActive {
			t.Fatalf("expected TradingSessionActive false")
		}
		if record.VWAP != nil || record.VAL != nil || record.VAH != nil {
			t.Fatalf("expected nil indicators, got %+v", record)
		}
	}
}

func TestService_Fill_SkipsNonTQBR(t *testing.T) {
	ctx := context.Background()

	infoRepo := &fakeTickerInfoRepo{items: []models.TickerInfo{{ID: 1, TickerName: "SBER", BoardID: "TQTF"}}}
	historyRepo := &fakeTickerHistoryRepo{}
	calc := &fakeCalculator{}

	now := time.Date(2025, 10, 21, 10, 0, 0, 0, time.UTC)

	svc, err := NewService(infoRepo, historyRepo, calc, 1, 3, WithNowFunc(func() time.Time { return now }), WithLocation(time.UTC))
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	if err := svc.Fill(ctx); err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	if calc.calls != 0 {
		t.Fatalf("expected calculator to not be called, got %d", calc.calls)
	}
	if len(historyRepo.inserted) != 0 {
		t.Fatalf("expected no inserts, got %d", len(historyRepo.inserted))
	}
}

func TestService_Fill_RespectsMaxInactive(t *testing.T) {
	ctx := context.Background()

	ticker := models.TickerInfo{ID: 5, TickerName: "LKOH", BoardID: boardTQBR}
	infoRepo := &fakeTickerInfoRepo{items: []models.TickerInfo{ticker}}
	historyRepo := &fakeTickerHistoryRepo{}
	calc := &fakeCalculator{err: indicators.ErrNoTrades}

	now := time.Date(2025, 10, 21, 10, 0, 0, 0, time.UTC)

	svc, err := NewService(infoRepo, historyRepo, calc, 2, 2, WithNowFunc(func() time.Time { return now }), WithLocation(time.UTC))
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	if err := svc.Fill(ctx); err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	if len(historyRepo.inserted) != 2 {
		t.Fatalf("expected 2 inserts (per maxInactive), got %d", len(historyRepo.inserted))
	}
}

func TestNewService_ValidatesInput(t *testing.T) {
	_, err := NewService(nil, nil, nil, 0, 0)
	if err == nil {
		t.Fatalf("expected error for nil dependencies")
	}

	infoRepo := &fakeTickerInfoRepo{}
	historyRepo := &fakeTickerHistoryRepo{}
	calc := &fakeCalculator{}

	_, err = NewService(infoRepo, historyRepo, calc, 0, 1)
	if err == nil {
		t.Fatalf("expected error for zero sessions limit")
	}

	_, err = NewService(infoRepo, historyRepo, calc, 1, 0)
	if err == nil {
		t.Fatalf("expected error for zero max inactive")
	}
}

type fakeTickerInfoRepo struct {
	items []models.TickerInfo
	err   error
}

func (f *fakeTickerInfoRepo) ListAll(ctx context.Context) ([]models.TickerInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]models.TickerInfo(nil), f.items...), nil
}

type fakeTickerHistoryRepo struct {
	records  map[string]map[string]models.TickerHistory
	inserted []models.TickerHistory
}

func (f *fakeTickerHistoryRepo) GetByDateAndName(ctx context.Context, name string, sessionDate time.Time) (models.TickerHistory, error) {
	if f.records == nil {
		return models.TickerHistory{}, db.ErrNotFound
	}
	byTicker := f.records[name]
	if byTicker == nil {
		return models.TickerHistory{}, db.ErrNotFound
	}
	if record, ok := byTicker[sessionDate.Format(time.DateOnly)]; ok {
		return record, nil
	}
	return models.TickerHistory{}, db.ErrNotFound
}

func (f *fakeTickerHistoryRepo) Insert(ctx context.Context, entity models.TickerHistory) error {
	f.inserted = append(f.inserted, entity)
	return nil
}

type fakeCalculator struct {
	profiles map[string]indicators.SessionProfile
	err      error
	calls    int
}

func (f *fakeCalculator) CalculateMainSessionProfile(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (indicators.SessionProfile, error) {
	f.calls++
	if f.err != nil {
		return indicators.SessionProfile{}, f.err
	}
	if f.profiles == nil {
		return indicators.SessionProfile{}, errors.New("profile not found")
	}
	profile, ok := f.profiles[sessionDate.Format(time.DateOnly)]
	if !ok {
		return indicators.SessionProfile{}, errors.New("profile not found")
	}
	return profile, nil
}
