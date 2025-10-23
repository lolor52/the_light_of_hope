package tickers_filling

import (
	"context"
	"encoding/json"
	"log"
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
		log.Printf("tickers_filling: request %s %s", r.Method, r.URL.Path)
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			log.Printf("tickers_filling: method not allowed %s", r.Method)
			return
		}

		stats, err := svc.Fill(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("tickers_filling: fill failed: %v", err)
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
		log.Printf("tickers_filling: response existing=%d created=%d active=%d inactive=%d", response.ExistingRecords, response.CreatedRecords, response.ActiveSessions, response.InactiveSessions)
	})
}
