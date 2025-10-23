package main

import (
        "errors"
        "log"
        "net/http"
        "os"
)

func main() {
        mux := http.NewServeMux()

        addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("HTTP сервер запущен на %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("ошибка запуска HTTP сервера: %v", err)
	}
}
