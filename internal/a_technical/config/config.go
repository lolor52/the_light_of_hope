package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config содержит общие параметры приложения.
type Config struct {
	DatabaseURL  string              `json:"DATABASE_URL"`
	MoexPassport PassportCredentials `json:"moex_passport"`
	// TickersFillingSessions определяет количество активных сессий для модуля заполнения истории.
	TickersFillingSessions int `json:"tickers_filling_sessions"`
	// TickersFillingMaxInactiveDays ограничивает количество проверяемых дат без активных сессий.
	TickersFillingMaxInactiveDays int `json:"tickers_filling_max_inactive_days"`
}

// PassportCredentials описывают логин и пароль для MOEX Passport.
type PassportCredentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
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
