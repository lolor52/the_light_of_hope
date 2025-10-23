package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"invest_intraday/models"
)

// TickerRepository инкапсулирует операции с таблицей ticker_history.
type TickerRepository struct {
	db *sql.DB
}

// NewTickerRepository создаёт репозиторий на базе готового подключения.
func NewTickerRepository(db *sql.DB) *TickerRepository {
	return &TickerRepository{db: db}
}

// ErrNotFound возвращается, если запись для тикера и даты отсутствует.
var ErrNotFound = errors.New("ticker_history entry not found")

// GetByDateAndName ищет запись по имени тикера и дате торговой сессии.
func (r *TickerRepository) GetByDateAndName(ctx context.Context, name string, sessionDate time.Time) (models.TickerHistory, error) {
	const query = `
SELECT th.id,
       th.trading_session_date,
       th.trading_session_active,
       th.ticker_info_id,
       th.vwap,
       th.val,
       th.vah,
       th.swing_count_paired
  FROM ticker_history th
  JOIN ticker_info ti ON ti.id = th.ticker_info_id
 WHERE ti.ticker_name = $1
   AND th.trading_session_date = $2
`

	var entity models.TickerHistory
	err := r.db.QueryRowContext(ctx, query, name, sessionDate).Scan(
		&entity.ID,
		&entity.TradingSessionDate,
		&entity.TradingSessionActive,
		&entity.TickerInfoID,
		&entity.VWAP,
		&entity.VAL,
		&entity.VAH,
		&entity.SwingCountPaired,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.TickerHistory{}, ErrNotFound
	}
	if err != nil {
		return models.TickerHistory{}, fmt.Errorf("select ticker_history: %w", err)
	}

	return entity, nil
}

// Insert добавляет новую запись о торговой сессии тикера.
func (r *TickerRepository) Insert(ctx context.Context, entity models.TickerHistory) error {
	const query = `
INSERT INTO ticker_history (
    trading_session_date,
    trading_session_active,
    ticker_info_id,
    vwap,
    val,
    vah,
    swing_count_paired
) VALUES ($1,$2,$3,$4,$5,$6,$7)
`

	_, err := r.db.ExecContext(ctx, query,
		entity.TradingSessionDate,
		entity.TradingSessionActive,
		entity.TickerInfoID,
		entity.VWAP,
		entity.VAL,
		entity.VAH,
		entity.SwingCountPaired,
	)
	if err != nil {
		return fmt.Errorf("insert ticker_history: %w", err)
	}

	return nil
}

// ListLastActiveSessions возвращает последние активные торговые сессии указанного тикера.
func (r *TickerRepository) ListLastActiveSessions(ctx context.Context, name string, limit int) ([]models.TickerHistory, error) {
	if limit <= 0 {
		return nil, nil
	}

	const query = `
SELECT th.id,
       th.trading_session_date,
       th.trading_session_active,
       th.ticker_info_id,
       th.vwap,
       th.val,
       th.vah,
       th.swing_count_paired
  FROM ticker_history th
  JOIN ticker_info ti ON ti.id = th.ticker_info_id
 WHERE ti.ticker_name = $1
   AND th.trading_session_active = true
 ORDER BY th.trading_session_date DESC
 LIMIT $2
`

	rows, err := r.db.QueryContext(ctx, query, name, limit)
	if err != nil {
		return nil, fmt.Errorf("list ticker_history sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]models.TickerHistory, 0, limit)
	for rows.Next() {
		var entity models.TickerHistory
		if err := rows.Scan(
			&entity.ID,
			&entity.TradingSessionDate,
			&entity.TradingSessionActive,
			&entity.TickerInfoID,
			&entity.VWAP,
			&entity.VAL,
			&entity.VAH,
			&entity.SwingCountPaired,
		); err != nil {
			return nil, fmt.Errorf("scan ticker_history session: %w", err)
		}
		sessions = append(sessions, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticker_history sessions: %w", err)
	}

	for i, j := 0, len(sessions)-1; i < j; i, j = i+1, j-1 {
		sessions[i], sessions[j] = sessions[j], sessions[i]
	}

	return sessions, nil
}
