package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// PassportCredentials содержит логин и пароль для MOEX Passport.
type PassportCredentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// Config агрегирует необходимые параметры запуска модуля выбора тикеров.
type Config struct {
	MOEXPassport                  PassportCredentials `json:"moex_passport"`
	DatabaseURL                   string              `json:"DATABASE_URL"`
	TickersFillingSessions        int                 `json:"tickers_filling_sessions"`
	TickersFillingMaxInactiveDays int                 `json:"tickers_filling_max_inactive_days"`
	TickersSelectionCount         int                 `json:"tickers_selection_count"`
}

// FromFile загружает конфигурацию из указанного JSON-файла.
func FromFile(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("open config: %w", err)
	}

	cleaned, err := removeJSONComments(data)
	if err != nil {
		return cfg, fmt.Errorf("prepare config: %w", err)
	}

	if err := json.Unmarshal(cleaned, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}

	if cfg.TickersFillingSessions <= 0 {
		cfg.TickersFillingSessions = 5
	}
	if cfg.TickersFillingMaxInactiveDays <= 0 {
		cfg.TickersFillingMaxInactiveDays = 20
	}
	if cfg.TickersSelectionCount <= 0 {
		cfg.TickersSelectionCount = 4
	}

	return cfg, nil
}

func removeJSONComments(data []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var builder strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		builder.WriteString(line)
		builder.WriteByte('\n')
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return []byte(builder.String()), nil
}
