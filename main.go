package main

import (
	"errors"
	"log"
	"net/http"
	"os"

	"invest_intraday/internal/a_submodule/tickers_filling"
	"invest_intraday/internal/a_technical/config"
	"invest_intraday/internal/tickers_selection"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.json"
	}

	cfg, err := config.FromFile(cfgPath)
	if err != nil {
		log.Fatalf("загрузка конфигурации: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/tickers_filling", tickers_filling.NewHTTPHandler(cfg))
	mux.Handle("/tickers_selection", tickers_selection.NewHTTPHandler(cfg))

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("HTTP сервер запущен на %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("ошибка запуска HTTP сервера: %v", err)
	}
}
