package alor

import (
	"encoding/json"
	"errors"
	"net/http"
)

type checkResponse struct {
	Authorized bool   `json:"authorized"`
	Message    string `json:"message,omitempty"`
}

// NewCheckHandler возвращает HTTP-обработчик для проверки авторизации в Alor.
func NewCheckHandler(checker AuthorizationChecker) http.Handler {
	if checker == nil {
		panic("alor: authorization checker is nil")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}

		if err := checker.CheckAuthorization(r.Context()); err != nil {
			if errors.Is(err, ErrUnauthorized) {
				writeJSON(w, http.StatusOK, checkResponse{Authorized: false, Message: "требуется повторная авторизация"})
				return
			}

			writeJSON(w, http.StatusBadGateway, checkResponse{Authorized: false, Message: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, checkResponse{Authorized: true})
	})
}

func writeJSON(w http.ResponseWriter, status int, payload checkResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
