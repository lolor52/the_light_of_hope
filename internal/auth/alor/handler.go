package alor

import (
	"context"
	"encoding/json"
	"net/http"
)

// AuthorizationChecker описывает функциональность проверки авторизации в Alor.
type AuthorizationChecker interface {
	CheckAuthorization(ctx context.Context) (Status, error)
}

// NewCheckHandler создаёт HTTP-обработчик для эндпоинта POST /auth/alor/check.
func NewCheckHandler(checker AuthorizationChecker) http.Handler {
	if checker == nil {
		panic("alor: отсутствует реализация AuthorizationChecker")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}

		status, err := checker.CheckAuthorization(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}
