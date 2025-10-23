package tickers_filling

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubFillService struct {
	summary Summary
	err     error
	calls   int
}

func (s *stubFillService) Fill(ctx context.Context) (Summary, error) {
	s.calls++
	return s.summary, s.err
}

func TestNewFillHandlerRejectsNonPost(t *testing.T) {
	handler := NewFillHandler(&stubFillService{})

	req := httptest.NewRequest(http.MethodGet, "/tickers_filling/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	if allow := rr.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("unexpected Allow header: %s", allow)
	}
}

func TestNewFillHandlerReturnsSummary(t *testing.T) {
	handler := NewFillHandler(&stubFillService{summary: Summary{
		ExistingRecords:  3,
		CreatedRecords:   2,
		ActiveSessions:   4,
		InactiveSessions: 1,
	}})

	req := httptest.NewRequest(http.MethodPost, "/tickers_filling/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rr.Code)
	}

	var resp fillResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ExistingRecords != 3 || resp.CreatedRecords != 2 || resp.ActiveSessions != 4 || resp.InactiveSessions != 1 {
		t.Fatalf("unexpected payload: %+v", resp)
	}
}

func TestNewFillHandlerReturnsError(t *testing.T) {
	handler := NewFillHandler(&stubFillService{err: errors.New("boom")})

	req := httptest.NewRequest(http.MethodPost, "/tickers_filling/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", rr.Code)
	}

	var resp errorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != "boom" {
		t.Fatalf("unexpected error message: %s", resp.Error)
	}
}
