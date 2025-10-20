package models

import "time"

// Ticker описывает агрегированные метрики торговой сессии конкретного тикера.
type Ticker struct {
	TradingSessionDate   time.Time `db:"trading_session_date"`
	TradingSessionActive *bool     `db:"trading_session_active"`
	Ticker               string    `db:"ticker"`
	VWAP                 *string   `db:"vwap"`
	VAL                  *string   `db:"val"`
	VAH                  *string   `db:"vah"`
	Liquidity            *string   `db:"liquidity"`
	Volatility           *string   `db:"volatility"`
	FlatTrendFilter      *string   `db:"flat_trend_filter"`
}
