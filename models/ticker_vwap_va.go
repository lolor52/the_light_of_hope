package models

import "time"

// TickerVWAPVA описывает данные VWAP/VAL/VAH для торговой сессии конкретного тикера.
type TickerVWAPVA struct {
	TradingSessionDate   time.Time `db:"trading_session_date"`
	TradingSessionActive *bool     `db:"trading_session_active"`
	Ticker               string    `db:"ticker"`
	VWAP                 *string   `db:"vwap"`
	VAL                  *string   `db:"val"`
	VAH                  *string   `db:"vah"`
}
