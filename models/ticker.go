package models

import "time"

// Ticker описывает агрегированные метрики торговой сессии конкретного тикера.
type Ticker struct {
	// TradingSessionDate определяет календарный день, к которому относятся сохранённые показатели.
	TradingSessionDate time.Time `db:"trading_session_date"`
	// TradingSessionActive хранит флаг о том, открыта ли сессия, чтобы интерпретировать неполные данные.
	TradingSessionActive bool `db:"trading_session_active"`
	// TickerName содержит биржевой идентификатор инструмента, для которого агрегированы метрики.
	TickerName string `db:"ticker_name"`
	// SecID хранит уникальный идентификатор инструмента в торговой системе для точного сопоставления данных.
	SecID string `db:"secid"`
	// BoardID фиксирует торговый режим площадки, необходимый для выборки данных по правильному рынку.
	BoardID string `db:"boardid"`
	// VWAP удерживает значение средней цены сделки, позволяя оценивать уровень спроса за сессию.
	VWAP *string `db:"vwap"`
	// VAL фиксирует нижнюю границу справедливого ценового диапазона, используемую в сигналах.
	VAL *string `db:"val"`
	// VAH фиксирует верхнюю границу справедливого ценового диапазона, используемую в сигналах.
	VAH *string `db:"vah"`
	// Liquidity описывает рассчитанный объём ликвидности, помогающий фильтровать неликвидные активы.
	Liquidity *string `db:"liquidity"`
	// Volatility хранит оценку волатильности, необходимую для подбора торговой тактики.
	Volatility *string `db:"volatility"`
	// FlatTrendFilter содержит индикатор для отсечения боковых трендов при генерации стратегий.
	FlatTrendFilter *string `db:"flat_trend_filter"`
}
