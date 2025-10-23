package alor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubChecker struct {
	err       error
	callCount int
}

func (s *stubChecker) CheckAuthorization(_ context.Context) error {
	s.callCount++
	return s.err
}

func TestNewCheckHandlerSuccess(t *testing.T) {
	checker := &stubChecker{}
	req := httptest.NewRequest(http.MethodPost, "/auth/alor/check", nil)
	w := httptest.NewRecorder()

	handler := NewCheckHandler(checker)
	handler.ServeHTTP(w, req)

	if checker.callCount != 1 {
		t.Fatalf("expected 1 call, got %d", checker.callCount)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Code)
	}

	expectedBody := "{\"authorized\":true}\n"
	if w.Body.String() != expectedBody {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestNewCheckHandlerUnauthorized(t *testing.T) {
	checker := &stubChecker{err: ErrUnauthorized}
	req := httptest.NewRequest(http.MethodPost, "/auth/alor/check", nil)
	w := httptest.NewRecorder()

	handler := NewCheckHandler(checker)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Code)
	}

	expected := "{\"authorized\":false,\"message\":\"требуется повторная авторизация\"}\n"
	if w.Body.String() != expected {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestNewCheckHandlerUpstreamError(t *testing.T) {
	checker := &stubChecker{err: errors.New("upstream failure")}
	req := httptest.NewRequest(http.MethodPost, "/auth/alor/check", nil)
	w := httptest.NewRecorder()

	handler := NewCheckHandler(checker)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status: %d", w.Code)
	}

	expected := "{\"authorized\":false,\"message\":\"upstream failure\"}\n"
	if w.Body.String() != expected {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestNewCheckHandlerMethodNotAllowed(t *testing.T) {
	checker := &stubChecker{}
	req := httptest.NewRequest(http.MethodGet, "/auth/alor/check", nil)
	w := httptest.NewRecorder()

	handler := NewCheckHandler(checker)
	handler.ServeHTTP(w, req)

	if checker.callCount != 0 {
		t.Fatalf("expected 0 calls, got %d", checker.callCount)
	}

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}
