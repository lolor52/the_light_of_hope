package db

import (
	"context"
	"database/sql"
	"fmt"

	"invest_intraday/models"
)

// TickerInfoRepository предоставляет доступ к справочным данным тикеров.
type TickerInfoRepository struct {
	db *sql.DB
}

// NewTickerInfoRepository создаёт репозиторий для таблицы ticker_info.
func NewTickerInfoRepository(db *sql.DB) *TickerInfoRepository {
	return &TickerInfoRepository{db: db}
}

// List возвращает полный список тикеров из таблицы ticker_info.
func (r *TickerInfoRepository) List(ctx context.Context) ([]models.TickerInfo, error) {
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
		return nil, fmt.Errorf("select ticker_info: %w", err)
	}
	defer rows.Close()

	var items []models.TickerInfo
	for rows.Next() {
		var info models.TickerInfo
		if err := rows.Scan(
			&info.ID,
			&info.TickerName,
			&info.SecID,
			&info.BoardID,
		); err != nil {
			return nil, fmt.Errorf("scan ticker_info: %w", err)
		}
		items = append(items, info)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticker_info: %w", err)
	}

	return items, nil
}
