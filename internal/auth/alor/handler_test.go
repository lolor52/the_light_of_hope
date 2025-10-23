package alor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type stubChecker struct {
	status Status
	err    error
	calls  int
}

func (s *stubChecker) CheckAuthorization(ctx context.Context) (Status, error) {
	s.calls++
	return s.status, s.err
}

func TestNewCheckHandlerSuccess(t *testing.T) {
	t.Parallel()

	checker := &stubChecker{
		status: Status{Authorized: true, ExpiresAt: time.Now().Add(5 * time.Minute)},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/alor/check", nil)

	handler := NewCheckHandler(checker)
	handler.ServeHTTP(rr, req)

	if checker.calls != 1 {
		t.Fatalf("ожидался один вызов CheckAuthorization, получено %d", checker.calls)
	}

	if rr.Code != http.StatusOK {
		t.Fatalf("ожидался статус 200, получено %d", rr.Code)
	}

	var resp Status
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ответ не является JSON: %v", err)
	}
	if !resp.Authorized {
		t.Fatalf("ожидалась авторизация в ответе")
	}
}

func TestNewCheckHandlerMethodNotAllowed(t *testing.T) {
	t.Parallel()

	checker := &stubChecker{}
	handler := NewCheckHandler(checker)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/alor/check", nil)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("ожидался статус 405, получено %d", rr.Code)
	}
	if rr.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("заголовок Allow не установлен")
	}
	if checker.calls != 0 {
		t.Fatalf("CheckAuthorization не должен вызываться")
	}
}

func TestNewCheckHandlerError(t *testing.T) {
	t.Parallel()

	checker := &stubChecker{
		err: context.DeadlineExceeded,
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/alor/check", nil)

	handler := NewCheckHandler(checker)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("ожидался статус 502, получено %d", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("ответ не является JSON: %v", err)
	}
	if resp["error"] == "" {
		t.Fatalf("ожидалось сообщение об ошибке")
	}
}
