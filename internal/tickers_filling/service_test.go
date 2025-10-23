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

type fakeInfoRepo struct {
	items []models.TickerInfo
	err   error
}

func (f *fakeInfoRepo) ListAll(context.Context) ([]models.TickerInfo, error) {
	return f.items, f.err
}

type fakeHistoryRepo struct {
	entries map[string]models.TickerHistory
	names   map[int64]string
	errGet  error
	errSave error
}

func newFakeHistoryRepo() *fakeHistoryRepo {
	return &fakeHistoryRepo{
		entries: make(map[string]models.TickerHistory),
		names:   make(map[int64]string),
	}
}

func (f *fakeHistoryRepo) key(name string, date time.Time) string {
	return name + ":" + date.Format("2006-01-02")
}

func (f *fakeHistoryRepo) GetByDateAndName(ctx context.Context, name string, sessionDate time.Time) (models.TickerHistory, error) {
	if f.errGet != nil {
		return models.TickerHistory{}, f.errGet
	}

	if entity, ok := f.entries[f.key(name, sessionDate)]; ok {
		return entity, nil
	}

	return models.TickerHistory{}, db.ErrNotFound
}

func (f *fakeHistoryRepo) Insert(ctx context.Context, entity models.TickerHistory) error {
	if f.errSave != nil {
		return f.errSave
	}

	name, ok := f.names[entity.TickerInfoID]
	if !ok {
		return errors.New("test: ticker name missing")
	}

	f.entries[f.key(name, entity.TradingSessionDate)] = entity

	return nil
}

type fakeCalculator struct {
	responses map[string]calcResult
}

type calcResult struct {
	profile indicators.SessionProfile
	err     error
}

func newFakeCalculator() *fakeCalculator {
	return &fakeCalculator{responses: make(map[string]calcResult)}
}

func (f *fakeCalculator) CalculateMainSessionProfile(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (indicators.SessionProfile, error) {
	key := sessionDate.Format("2006-01-02")
	if res, ok := f.responses[key]; ok {
		return res.profile, res.err
	}
	return indicators.SessionProfile{}, errors.New("test: unexpected date")
}

func TestServiceFillCreatesActiveSessions(t *testing.T) {
	repoInfo := &fakeInfoRepo{items: []models.TickerInfo{{ID: 1, TickerName: "GAZP", SecID: "GAZP", BoardID: "TQBR"}}}
	historyRepo := newFakeHistoryRepo()
	historyRepo.names[1] = "GAZP"
	calc := newFakeCalculator()

	moscow, _ := time.LoadLocation("Europe/Moscow")
	calc.responses["2025-10-20"] = calcResult{profile: indicators.SessionProfile{VWAP: 100.5, VAL: 99.1, VAH: 101.2}}
	calc.responses["2025-10-19"] = calcResult{profile: indicators.SessionProfile{VWAP: 101.1, VAL: 100.0, VAH: 102.3}}

	svc, err := NewService(repoInfo, historyRepo, calc, 2, 5)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}
	svc.now = func() time.Time {
		return time.Date(2025, 10, 21, 12, 0, 0, 0, moscow)
	}

	if err := svc.Fill(context.Background()); err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	if len(historyRepo.entries) != 2 {
		t.Fatalf("unexpected number of entries: %d", len(historyRepo.entries))
	}

	entry1, ok := historyRepo.entries["GAZP:2025-10-20"]
	if !ok {
		t.Fatalf("entry for 2025-10-20 not found")
	}
	if !entry1.TradingSessionActive {
		t.Fatalf("expected session active")
	}
	if entry1.VWAP == nil || *entry1.VWAP != "100.5" {
		t.Fatalf("unexpected VWAP: %v", entry1.VWAP)
	}
	if entry1.VAL == nil || *entry1.VAL != "99.1" {
		t.Fatalf("unexpected VAL: %v", entry1.VAL)
	}
	if entry1.VAH == nil || *entry1.VAH != "101.2" {
		t.Fatalf("unexpected VAH: %v", entry1.VAH)
	}

	entry2, ok := historyRepo.entries["GAZP:2025-10-19"]
	if !ok {
		t.Fatalf("entry for 2025-10-19 not found")
	}
	if !entry2.TradingSessionActive {
		t.Fatalf("expected second session active")
	}
}

func TestServiceFillCreatesInactiveSession(t *testing.T) {
	repoInfo := &fakeInfoRepo{items: []models.TickerInfo{{ID: 1, TickerName: "SBER", SecID: "SBER", BoardID: "TQBR"}}}
	historyRepo := newFakeHistoryRepo()
	historyRepo.names[1] = "SBER"
	calc := newFakeCalculator()

	moscow, _ := time.LoadLocation("Europe/Moscow")
	calc.responses["2025-10-20"] = calcResult{err: indicators.ErrNoTrades}
	calc.responses["2025-10-19"] = calcResult{profile: indicators.SessionProfile{VWAP: 200.1, VAL: 198.4, VAH: 201.7}}

	svc, err := NewService(repoInfo, historyRepo, calc, 1, 3)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}
	svc.now = func() time.Time {
		return time.Date(2025, 10, 21, 15, 0, 0, 0, moscow)
	}

	if err := svc.Fill(context.Background()); err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	inactive, ok := historyRepo.entries["SBER:2025-10-20"]
	if !ok {
		t.Fatalf("inactive day not stored")
	}
	if inactive.TradingSessionActive {
		t.Fatalf("expected inactive session")
	}
	if inactive.VWAP != nil || inactive.VAL != nil || inactive.VAH != nil {
		t.Fatalf("expected no indicators for inactive session")
	}

	active, ok := historyRepo.entries["SBER:2025-10-19"]
	if !ok {
		t.Fatalf("active day not stored")
	}
	if !active.TradingSessionActive {
		t.Fatalf("expected active session")
	}
}

func TestServiceFillSkipsExistingHistory(t *testing.T) {
	repoInfo := &fakeInfoRepo{items: []models.TickerInfo{{ID: 1, TickerName: "LKOH", SecID: "LKOH", BoardID: "TQBR"}}}
	historyRepo := newFakeHistoryRepo()
	historyRepo.names[1] = "LKOH"
	existingDate, _ := time.Parse("2006-01-02", "2025-10-20")
	historyRepo.entries["LKOH:2025-10-20"] = models.TickerHistory{TradingSessionDate: existingDate, TradingSessionActive: true}
	calc := newFakeCalculator()
	calc.responses["2025-10-19"] = calcResult{profile: indicators.SessionProfile{VWAP: 300.3, VAL: 299.1, VAH: 301.5}}

	moscow, _ := time.LoadLocation("Europe/Moscow")

	svc, err := NewService(repoInfo, historyRepo, calc, 2, 4)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}
	svc.now = func() time.Time {
		return time.Date(2025, 10, 21, 10, 0, 0, 0, moscow)
	}

	if err := svc.Fill(context.Background()); err != nil {
		t.Fatalf("Fill returned error: %v", err)
	}

	if len(historyRepo.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(historyRepo.entries))
	}
}
