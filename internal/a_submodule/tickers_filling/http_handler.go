package tickers_filling

import (
	"encoding/json"
	"log"
	"net/http"
)

type httpResponse struct {
	ExistingRecords      int `json:"existing_records"`
	CreatedRecords       int `json:"created_records"`
	ActiveSessionDates   int `json:"active_session_dates"`
	InactiveSessionDates int `json:"inactive_session_dates"`
}

// NewHTTPHandler возвращает HTTP-обработчик, запускающий заполнение истории тикеров.
func NewHTTPHandler(service *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}

		stats, err := service.Fill(r.Context())
		if err != nil {
			log.Printf("tickers_filling: ошибка выполнения: %v", err)
			http.Error(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
			return
		}

		response := httpResponse{
			ExistingRecords:      stats.ExistingRecords,
			CreatedRecords:       stats.CreatedRecords,
			ActiveSessionDates:   stats.ActiveSessionDates,
			InactiveSessionDates: stats.InactiveSessionDates,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("tickers_filling: ошибка сериализации ответа: %v", err)
		}
	})
}
