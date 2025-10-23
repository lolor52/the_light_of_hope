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
	tickersdb "invest_intraday/internal/a_technical/db"
	"invest_intraday/internal/auth/alor"
	"invest_intraday/internal/indicators"
	"invest_intraday/internal/tickers_filling"
)

func main() {
	cfg, err := loadAppConfig()
	if err != nil {
		log.Fatalf("не удалось загрузить конфигурацию: %v", err)
	}

	alorClient, err := alor.NewClient(cfg.AlorRefreshToken)
	if err != nil {
		log.Fatalf("ошибка инициализации клиента Alor: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = cfg.DatabaseURL
	}
	if strings.TrimSpace(databaseURL) == "" {
		log.Fatal("DATABASE_URL не задан ни в окружении, ни в конфигурации")
	}

	sqlDB, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("ошибка открытия соединения с БД: %v", err)
	}
	defer func() {
		if cerr := sqlDB.Close(); cerr != nil {
			log.Printf("ошибка закрытия соединения с БД: %v", cerr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		log.Fatalf("ошибка подключения к БД: %v", err)
	}

	tickerInfoRepo := tickersdb.NewTickerInfoRepository(sqlDB)
	tickerHistoryRepo := tickersdb.NewTickerRepository(sqlDB)

	marketDataClient := indicators.NewMarketDataClient(alor.ProdEnvironment.APIURL, alorClient)
	calculator := indicators.NewValueAreaCalculator(tickerInfoRepo, marketDataClient)

	fillingService, err := tickers_filling.NewService(
		tickerInfoRepo,
		tickerHistoryRepo,
		calculator,
		cfg.TickersFillingSessions,
		cfg.TickersFillingMaxInactiveDays,
	)
	if err != nil {
		log.Fatalf("ошибка инициализации сервиса заполнения тикеров: %v", err)
	}

	fillHandler := tickers_filling.NewFillHandler(fillingService)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.Handle("/auth/alor/check", alor.NewCheckHandler(alorClient))
	mux.Handle("/tickers_filling", fillHandler)
	mux.Handle("/tickers_filling/", fillHandler)

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
