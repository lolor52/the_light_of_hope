package tickers_filling

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"invest_intraday/internal/a_technical/config"
)

// HTTPHandler обрабатывает HTTP-запросы для запуска заполнения тикеров.
type HTTPHandler struct {
	cfg config.Config
}

// NewHTTPHandler создаёт HTTP-обработчик с заранее загруженной конфигурацией.
func NewHTTPHandler(cfg config.Config) *HTTPHandler {
	return &HTTPHandler{cfg: cfg}
}

// ServeHTTP поддерживает только POST-запросы и запускает сервис заполнения тикеров.
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	service, err := NewService(ctx, h.cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer service.Close()

	if err := service.Run(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
