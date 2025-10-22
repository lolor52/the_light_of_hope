package models

import "time"

// OrderPrice описывает сохранённые уровни входа для заявок.
type OrderPrice struct {
	// ID хранит первичный ключ записи в таблице order_price.
	ID int64 `db:"id"`
	// TickerID содержит идентификатор связанного тикера.
	TickerID int64 `db:"tickers_id"`
	// PriceLong фиксирует цену входа в длинную позицию.
	PriceLong float64 `db:"price_long"`
	// PriceShort фиксирует цену входа в короткую позицию.
	PriceShort float64 `db:"price_short"`
	// VWAP содержит расчёт среднего объёма-взвешенного уровня.
	VWAP string `db:"vwap"`
	// VAL хранит нижнюю границу справедливого диапазона.
	VAL string `db:"val"`
	// VAH хранит верхнюю границу справедливого диапазона.
	VAH string `db:"vah"`
	// DateTime определяет момент фиксации уровня цены.
	DateTime time.Time `db:"date_time"`
}
