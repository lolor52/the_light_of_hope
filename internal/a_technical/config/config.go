package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// MOEXTicker описывает параметры инструмента для сбора данных через MOEX ISS.
type MOEXTicker struct {
	Ticker  string `json:"ticker"`
	SecID   string `json:"SecID"`
	BoardID string `json:"BOARDID"`
}

// PassportCredentials содержит логин и пароль для MOEX Passport.
type PassportCredentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// Config агрегирует необходимые параметры запуска модуля выбора тикеров.
type Config struct {
	MOEXPassport           PassportCredentials `json:"moex_passport"`
	MOEXTickers            []MOEXTicker        `json:"moex_tickers_secid_boardid"`
	DatabaseURL            string              `json:"DATABASE_URL"`
	AlorToken              string              `json:"alor_token"`
	TickersFillingSessions int                 `json:"tickers_filling_sessions"`
}

// FromFile загружает конфигурацию из указанного JSON-файла.
func FromFile(path string) (Config, error) {
	var cfg Config

	file, err := os.Open(path)
	if err != nil {
		return cfg, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}

	if cfg.TickersFillingSessions <= 0 {
		cfg.TickersFillingSessions = 5
	}

	return cfg, nil
}
