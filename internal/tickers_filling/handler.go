package tickers_filling

import (
	"context"
	"encoding/json"
	"net/http"
)

type filler interface {
	Fill(ctx context.Context) (FillStats, error)
}

type fillResponse struct {
	ExistingRecords  int `json:"existing_records"`
	CreatedRecords   int `json:"created_records"`
	ActiveSessions   int `json:"active_sessions"`
	InactiveSessions int `json:"inactive_sessions"`
}

// NewFillHandler возвращает HTTP-обработчик, запускающий заполнение истории тикеров.
func NewFillHandler(svc filler) http.Handler {
	if svc == nil {
		panic("tickers_filling: service is nil")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}

		stats, err := svc.Fill(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		response := fillResponse{
			ExistingRecords:  stats.ExistingEntries,
			CreatedRecords:   stats.CreatedEntries,
			ActiveSessions:   stats.ActiveSessions,
			InactiveSessions: stats.InactiveSessions,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	})
}
