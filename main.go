package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"invest_intraday/internal/a_technical/config"
	"invest_intraday/internal/auth/alor"
)

func main() {
	cfg, err := loadAppConfig()
	if err != nil {
		log.Fatalf("не удалось загрузить конфигурацию: %v", err)
	}

	alorEnv := detectAlorEnvironment()
	alorClient, err := alor.NewClient(cfg.AlorRefreshToken, alor.WithEnvironment(alorEnv))
	if err != nil {
		log.Fatalf("ошибка инициализации клиента Alor: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.Handle("/auth/alor/check", alor.NewCheckHandler(alorClient))

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("HTTP сервер запущен на %s (функционал отключён)", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("ошибка запуска HTTP сервера: %v", err)
	}
}

func loadAppConfig() (config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.json"
	}

	cfg, err := config.FromFile(cfgPath)
	if err != nil {
		return config.Config{}, err
	}

	return cfg, nil
}

func detectAlorEnvironment() alor.Environment {
	switch strings.ToLower(os.Getenv("ALOR_ENV")) {
	case "dev", "test":
		return alor.DevEnvironment
	default:
		return alor.ProdEnvironment
	}
}
