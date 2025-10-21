package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"invest_intraday/models"
)

// TickerRepository инкапсулирует операции с таблицей ticker.
type TickerRepository struct {
	db *sql.DB
}

// NewTickerRepository создаёт репозиторий на базе готового подключения.
func NewTickerRepository(db *sql.DB) *TickerRepository {
	return &TickerRepository{db: db}
}

// ErrNotFound возвращается, если запись для тикера и даты отсутствует.
var ErrNotFound = errors.New("ticker entry not found")

// GetByDateAndName ищет запись по имени тикера и дате торговой сессии.
func (r *TickerRepository) GetByDateAndName(ctx context.Context, name string, sessionDate time.Time) (models.Ticker, error) {
	const query = `
SELECT trading_session_date,
       trading_session_active,
       ticker_name,
       secid,
       boardid,
       vwap,
       val,
       vah,
       liquidity,
       volatility,
       flat_trend_filter
  FROM ticker
 WHERE ticker_name = $1
   AND trading_session_date = $2
`

	var entity models.Ticker
	err := r.db.QueryRowContext(ctx, query, name, sessionDate).Scan(
		&entity.TradingSessionDate,
		&entity.TradingSessionActive,
		&entity.TickerName,
		&entity.SecID,
		&entity.BoardID,
		&entity.VWAP,
		&entity.VAL,
		&entity.VAH,
		&entity.Liquidity,
		&entity.Volatility,
		&entity.FlatTrendFilter,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Ticker{}, ErrNotFound
	}
	if err != nil {
		return models.Ticker{}, fmt.Errorf("select ticker: %w", err)
	}

	return entity, nil
}

// Insert добавляет новую запись о торговой сессии тикера.
func (r *TickerRepository) Insert(ctx context.Context, entity models.Ticker) error {
	const query = `
INSERT INTO ticker (
    trading_session_date,
    trading_session_active,
    ticker_name,
    secid,
    boardid,
    vwap,
    val,
    vah,
    liquidity,
    volatility,
    flat_trend_filter
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
`

	_, err := r.db.ExecContext(ctx, query,
		entity.TradingSessionDate,
		entity.TradingSessionActive,
		entity.TickerName,
		entity.SecID,
		entity.BoardID,
		entity.VWAP,
		entity.VAL,
		entity.VAH,
		entity.Liquidity,
		entity.Volatility,
		entity.FlatTrendFilter,
	)
	if err != nil {
		return fmt.Errorf("insert ticker: %w", err)
	}

	return nil
}

// ListLastActiveSessions возвращает последние активные торговые сессии указанного тикера.
func (r *TickerRepository) ListLastActiveSessions(ctx context.Context, name string, limit int) ([]models.Ticker, error) {
	if limit <= 0 {
		return nil, nil
	}

	const query = `
SELECT trading_session_date,
       trading_session_active,
       ticker_name,
       secid,
       boardid,
       vwap,
       val,
       vah,
       liquidity,
       volatility,
       flat_trend_filter
  FROM ticker
 WHERE ticker_name = $1
   AND trading_session_active = true
 ORDER BY trading_session_date DESC
 LIMIT $2
`

	rows, err := r.db.QueryContext(ctx, query, name, limit)
	if err != nil {
		return nil, fmt.Errorf("list ticker sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]models.Ticker, 0, limit)
	for rows.Next() {
		var entity models.Ticker
		if err := rows.Scan(
			&entity.TradingSessionDate,
			&entity.TradingSessionActive,
			&entity.TickerName,
			&entity.SecID,
			&entity.BoardID,
			&entity.VWAP,
			&entity.VAL,
			&entity.VAH,
			&entity.Liquidity,
			&entity.Volatility,
			&entity.FlatTrendFilter,
		); err != nil {
			return nil, fmt.Errorf("scan ticker session: %w", err)
		}
		sessions = append(sessions, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticker sessions: %w", err)
	}

	for i, j := 0, len(sessions)-1; i < j; i, j = i+1, j-1 {
		sessions[i], sessions[j] = sessions[j], sessions[i]
	}

	return sessions, nil
}
