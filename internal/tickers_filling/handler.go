package tickers_filling

import (
	"context"
	"encoding/json"
	"net/http"
)

type fillService interface {
	Fill(ctx context.Context) (Summary, error)
}

// NewFillHandler возвращает HTTP-обработчик запуска заполнения истории тикеров.
func NewFillHandler(service fillService) http.Handler {
	if service == nil {
		panic("tickers_filling: fill service is nil")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}

		summary, err := service.Fill(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, fillResponse{
			ExistingRecords:  summary.ExistingRecords,
			CreatedRecords:   summary.CreatedRecords,
			ActiveSessions:   summary.ActiveSessions,
			InactiveSessions: summary.InactiveSessions,
		})
	})
}

type fillResponse struct {
	ExistingRecords  int `json:"existing_records"`
	CreatedRecords   int `json:"created_records"`
	ActiveSessions   int `json:"active_sessions"`
	InactiveSessions int `json:"inactive_sessions"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	message := "внутренняя ошибка"
	if err != nil {
		message = err.Error()
	}
	writeJSON(w, status, errorResponse{Error: message})
}
