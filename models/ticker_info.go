package models

// TickerInfo описывает справочную информацию о тикере.
type TickerInfo struct {
	// ID хранит первичный ключ записи в таблице ticker_info.
	ID int64 `db:"id"`
	// TickerName содержит биржевой идентификатор инструмента.
	TickerName string `db:"ticker_name"`
	// SecID хранит уникальный идентификатор инструмента во внутренней системе биржи.
	SecID string `db:"secid"`
	// BoardID указывает торговый режим площадки, необходимый для запросов к API.
	BoardID string `db:"boardid"`
}
