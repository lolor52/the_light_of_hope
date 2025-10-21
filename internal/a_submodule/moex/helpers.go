package moex

import (
	"fmt"
	"strconv"
	"time"
)

func columnIndex(columns []string) map[string]int {
	index := make(map[string]int, len(columns))
	for i, column := range columns {
		index[column] = i
	}
	return index
}

func floatFromRow(row []interface{}, idx int) float64 {
	if idx < 0 || idx >= len(row) {
		return 0
	}
	switch v := row[idx].(type) {
	case float64:
		return v
	case string:
		if v == "" {
			return 0
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

func parseDate(value interface{}) (time.Time, error) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return time.Time{}, fmt.Errorf("empty date value")
		}
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse date: %w", err)
		}
		return t, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported date type %T", value)
	}
}

func stringFromRow(row []interface{}, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	switch v := row[idx].(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
