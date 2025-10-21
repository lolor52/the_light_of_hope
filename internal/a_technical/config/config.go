package config

import (
	"bytes"
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
	TickersFillingSessions int                 `json:"tickers_filling_sessions"`
}

// FromFile загружает конфигурацию из указанного JSON-файла.
func FromFile(path string) (Config, error) {
	var cfg Config

	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}

	cleaned := stripJSONComments(raw)

	if err := json.Unmarshal(cleaned, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}

	if cfg.TickersFillingSessions <= 0 {
		cfg.TickersFillingSessions = 5
	}

	return cfg, nil
}

// stripJSONComments удаляет однострочные и многострочные комментарии из JSON-документа.
func stripJSONComments(src []byte) []byte {
	var (
		inString            bool
		inSingleLineComment bool
		inMultiLineComment  bool
		prev                byte
		buf                 bytes.Buffer
	)

	for i := 0; i < len(src); i++ {
		ch := src[i]
		var next byte
		if i+1 < len(src) {
			next = src[i+1]
		}

		if inSingleLineComment {
			if ch == '\n' || ch == '\r' {
				inSingleLineComment = false
				buf.WriteByte(ch)
			}
			continue
		}

		if inMultiLineComment {
			if ch == '*' && next == '/' {
				inMultiLineComment = false
				i++
			}
			continue
		}

		if inString {
			buf.WriteByte(ch)
			if ch == '"' && prev != '\\' {
				inString = false
			}
			prev = ch
			continue
		}

		switch {
		case ch == '"':
			inString = true
			buf.WriteByte(ch)
		case ch == '/' && next == '/':
			inSingleLineComment = true
			i++
		case ch == '/' && next == '*':
			inMultiLineComment = true
			i++
		case ch == '#':
			inSingleLineComment = true
		default:
			buf.WriteByte(ch)
		}

		prev = ch
	}

	return buf.Bytes()
}
