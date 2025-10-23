package tickers_filling

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubFiller struct {
	stats  FillStats
	err    error
	called bool
}

func (s *stubFiller) Fill(ctx context.Context) (FillStats, error) {
	s.called = true
	return s.stats, s.err
}

func TestNewFillHandlerSuccess(t *testing.T) {
	filler := &stubFiller{stats: FillStats{ExistingEntries: 1, CreatedEntries: 2, ActiveSessions: 3, InactiveSessions: 4}}

	handler := NewFillHandler(filler)

	req := httptest.NewRequest(http.MethodPost, "/tickers_filling/", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if !filler.called {
		t.Fatalf("expected service to be called")
	}

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.Code)
	}

	wantBody := "{\"existing_records\":1,\"created_records\":2,\"active_sessions\":3,\"inactive_sessions\":4}\n"
	if resp.Body.String() != wantBody {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}

	if resp.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: %s", resp.Header().Get("Content-Type"))
	}
}

func TestNewFillHandlerMethodNotAllowed(t *testing.T) {
	filler := &stubFiller{}
	handler := NewFillHandler(filler)

	req := httptest.NewRequest(http.MethodGet, "/tickers_filling/", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if filler.called {
		t.Fatalf("expected service not to be called")
	}

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status: %d", resp.Code)
	}

	if resp.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("unexpected Allow header: %s", resp.Header().Get("Allow"))
	}
}

func TestNewFillHandlerServiceError(t *testing.T) {
	filler := &stubFiller{err: errors.New("boom")}
	handler := NewFillHandler(filler)

	req := httptest.NewRequest(http.MethodPost, "/tickers_filling/", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", resp.Code)
	}

	if resp.Body.String() != "boom\n" {
		t.Fatalf("unexpected body: %s", resp.Body.String())
	}
}
