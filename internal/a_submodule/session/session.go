package session

import (
	"errors"
	"time"
)

// Boundaries описывает официальные и торговые границы сессии.
type Boundaries struct {
	OfficialStart time.Time
	OfficialEnd   time.Time
	TradingStart  time.Time
	TradingEnd    time.Time
}

const adjustment = time.Hour + 8*time.Minute

// CalculateBoundaries рассчитывает границы, используемые для торговли.
func CalculateBoundaries(officialStart, officialEnd time.Time) (Boundaries, error) {
	if officialEnd.Before(officialStart) {
		return Boundaries{}, errors.New("окончание сессии раньше начала")
	}

	tradingStart := officialStart.Add(adjustment)
	tradingEnd := officialEnd.Add(-adjustment)
	if tradingEnd.Before(tradingStart) {
		return Boundaries{}, errors.New("отрезок для торговли пуст")
	}

	return Boundaries{
		OfficialStart: officialStart,
		OfficialEnd:   officialEnd,
		TradingStart:  tradingStart,
		TradingEnd:    tradingEnd,
	}, nil
}
