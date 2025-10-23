package models

import "time"

// TickerHistory описывает агрегированные метрики торговой сессии конкретного тикера.
type TickerHistory struct {
	// ID хранит первичный ключ записи в таблице ticker_history.
	ID int64 `db:"id"`
	// TradingSessionDate определяет календарный день, к которому относятся сохранённые показатели.
	TradingSessionDate time.Time `db:"trading_session_date"`
	// TradingSessionActive хранит флаг о том, открыта ли сессия, чтобы интерпретировать неполные данные.
	TradingSessionActive bool `db:"trading_session_active"`
	// TickerInfoID содержит ссылку на справочную запись тикера в таблице ticker_info.
	TickerInfoID int64 `db:"ticker_info_id"`
	// VWAP удерживает значение средней цены сделки, позволяя оценивать уровень спроса за сессию.
	VWAP *string `db:"vwap"`
	// VAL фиксирует нижнюю границу справедливого ценового диапазона, используемую в сигналах.
	VAL *string `db:"val"`
	// VAH фиксирует верхнюю границу справедливого ценового диапазона, используемую в сигналах.
	VAH *string `db:"vah"`
	// SwingCountPaired хранит расчёт количества парных свингов для текущей сессии.
	SwingCountPaired *string `db:"swing_count_paired"`
}
