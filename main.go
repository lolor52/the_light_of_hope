package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"invest_intraday/internal/a_technical/config"
	"invest_intraday/internal/a_technical/db"
	"invest_intraday/internal/auth/alor"
	"invest_intraday/internal/indicators"
	"invest_intraday/internal/tickers_filling"
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

	database, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("ошибка подключения к базе данных: %v", err)
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := database.PingContext(ctx); err != nil {
		log.Fatalf("база данных недоступна: %v", err)
	}

	tickerInfoRepo := db.NewTickerInfoRepository(database)
	tickerHistoryRepo := db.NewTickerRepository(database)
	marketDataClient := indicators.NewMarketDataClient(alorEnv.APIURL, alorClient)
	valueAreaCalc := indicators.NewValueAreaCalculator(tickerInfoRepo, marketDataClient)

	fillingService, err := tickers_filling.NewService(
		tickerInfoRepo,
		tickerHistoryRepo,
		valueAreaCalc,
		cfg.TickersFillingSessions,
		cfg.TickersFillingMaxInactiveDays,
	)
	if err != nil {
		log.Fatalf("ошибка инициализации tickers_filling: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.Handle("/auth/alor/check", alor.NewCheckHandler(alorClient))
	mux.Handle("/tickers_filling/", tickers_filling.NewFillHandler(fillingService))

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("HTTP сервер запущен на %s", addr)

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
