package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"

	"invest_intraday/internal/a_submodule/alor"
	"invest_intraday/internal/a_submodule/indicators"
	"invest_intraday/internal/a_submodule/tickers_filling"
	"invest_intraday/internal/a_technical/config"
	dbpkg "invest_intraday/internal/a_technical/db"
)

func main() {
	cfg, err := loadAppConfig()
	if err != nil {
		log.Fatalf("не удалось загрузить конфигурацию: %v", err)
	}

	databaseURL := databaseURLFromEnv(cfg)
	if databaseURL == "" {
		log.Fatal("строка подключения к БД не задана")
	}

	db, err := openDatabase(databaseURL)
	if err != nil {
		log.Fatalf("не удалось подключиться к БД: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("ошибка закрытия соединения с БД: %v", cerr)
		}
	}()

	service, err := newTickersFillingService(db, cfg)
	if err != nil {
		log.Fatalf("не удалось создать сервис tickers_filling: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /tickers_filling", tickers_filling.NewHTTPHandler(service))

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("HTTP сервер запущен на %s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

func databaseURLFromEnv(cfg config.Config) string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	return cfg.DatabaseURL
}

func openDatabase(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("открытие подключения: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return db, nil
}

func newTickersFillingService(db *sql.DB, cfg config.Config) (*tickers_filling.Service, error) {
	tickerInfoRepo := dbpkg.NewTickerInfoRepository(db)
	historyRepo := dbpkg.NewTickerRepository(db)

	alorClient, err := alor.NewClient(cfg.AlorRefreshToken)
	if err != nil {
		return nil, fmt.Errorf("создание клиента ALOR: %w", err)
	}

	valueAreaCalc := indicators.NewCalculator(tickerInfoRepo, alorClient)
	service, err := tickers_filling.NewService(
		tickerInfoRepo,
		historyRepo,
		valueAreaCalc,
		tickers_filling.Config{
			SessionsTarget:  cfg.TickersFillingSessions,
			MaxInactiveDays: cfg.TickersFillingMaxInactiveDays,
		},
	)
	if err != nil {
		return nil, err
	}

	return service, nil
}
