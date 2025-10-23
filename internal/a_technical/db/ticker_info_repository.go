package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"invest_intraday/models"
)

// TickerInfoRepository инкапсулирует операции со справочными данными тикеров.
type TickerInfoRepository struct {
	db *sql.DB
}

// NewTickerInfoRepository создаёт репозиторий ticker_info на основе подключения к БД.
func NewTickerInfoRepository(db *sql.DB) *TickerInfoRepository {
	return &TickerInfoRepository{db: db}
}

// GetByID возвращает запись ticker_info по идентификатору.
func (r *TickerInfoRepository) GetByID(ctx context.Context, id int64) (models.TickerInfo, error) {
	const query = `
SELECT id,
       ticker_name,
       secid,
       boardid
  FROM ticker_info
 WHERE id = $1
`

	var entity models.TickerInfo
	if err := r.db.QueryRowContext(ctx, query, id).Scan(
		&entity.ID,
		&entity.TickerName,
		&entity.SecID,
		&entity.BoardID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.TickerInfo{}, fmt.Errorf("ticker_info %d: %w", id, err)
		}

		return models.TickerInfo{}, fmt.Errorf("select ticker_info %d: %w", id, err)
	}

	return entity, nil
}

// ListAll возвращает полный перечень тикеров из таблицы ticker_info.
func (r *TickerInfoRepository) ListAll(ctx context.Context) ([]models.TickerInfo, error) {
	const query = `
SELECT id,
       ticker_name,
       secid,
       boardid
  FROM ticker_info
 ORDER BY ticker_name
`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list ticker_info: %w", err)
	}
	defer rows.Close()

	var items []models.TickerInfo
	for rows.Next() {
		var entity models.TickerInfo
		if err := rows.Scan(
			&entity.ID,
			&entity.TickerName,
			&entity.SecID,
			&entity.BoardID,
		); err != nil {
			return nil, fmt.Errorf("scan ticker_info: %w", err)
		}
		items = append(items, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticker_info: %w", err)
	}

	return items, nil
}
