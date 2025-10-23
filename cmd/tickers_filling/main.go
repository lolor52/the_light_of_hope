package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
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
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("не удалось загрузить конфигурацию: %v", err)
	}

	databaseURL, err := resolveDatabaseURL(cfg)
	if err != nil {
		log.Fatalf("не удалось определить строку подключения к БД: %v", err)
	}

	if strings.TrimSpace(cfg.AlorRefreshToken) == "" {
		log.Fatal("в конфигурации отсутствует refresh-токен Alor")
	}

	dbConn, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("ошибка подключения к PostgreSQL: %v", err)
	}
	defer func() {
		if cerr := dbConn.Close(); cerr != nil {
			log.Printf("ошибка при закрытии соединения с БД: %v", cerr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := dbConn.PingContext(ctx); err != nil {
		log.Fatalf("PostgreSQL недоступен: %v", err)
	}

	infoRepo := db.NewTickerInfoRepository(dbConn)
	historyRepo := db.NewTickerRepository(dbConn)

	alorEnv := detectAlorEnvironment()
	alorClient, err := alor.NewClient(cfg.AlorRefreshToken, alor.WithEnvironment(alorEnv))
	if err != nil {
		log.Fatalf("ошибка инициализации клиента Alor: %v", err)
	}

	mdClient := indicators.NewMarketDataClient(alorEnv.APIURL, alorClient)
	calculator := indicators.NewValueAreaCalculator(infoRepo, mdClient)

	service, err := tickers_filling.NewService(
		infoRepo,
		historyRepo,
		calculator,
		cfg.TickersFillingSessions,
		cfg.TickersFillingMaxInactiveDays,
	)
	if err != nil {
		log.Fatalf("ошибка инициализации сервиса заполнения: %v", err)
	}

	if err := service.Fill(context.Background()); err != nil {
		log.Fatalf("ошибка заполнения истории тикеров: %v", err)
	}

	log.Println("заполнение истории тикеров успешно завершено")
}

func loadConfig() (config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.json"
	}
	return config.FromFile(cfgPath)
}

func resolveDatabaseURL(cfg config.Config) (string, error) {
	if env := strings.TrimSpace(os.Getenv("DATABASE_URL")); env != "" {
		return env, nil
	}
	if strings.TrimSpace(cfg.DatabaseURL) != "" {
		return cfg.DatabaseURL, nil
	}
	return "", errors.New("не задана переменная DATABASE_URL и отсутствует поле DATABASE_URL в конфигурации")
}

func detectAlorEnvironment() alor.Environment {
	switch strings.ToLower(os.Getenv("ALOR_ENV")) {
	case "dev", "test":
		return alor.DevEnvironment
	default:
		return alor.ProdEnvironment
	}
}
