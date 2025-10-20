package invariant

import (
	"fmt"

	"invest_intraday/internal/a_submodule/variable/metrics"
)

// Validate проверяет инвариант VAH > VWAP > VAL.
func Validate(m metrics.RangeMetrics) error {
	if !(m.VAH > m.VWAP && m.VWAP > m.VAL) {
		return fmt.Errorf("нарушен инвариант: VAH=%.4f, VWAP=%.4f, VAL=%.4f", m.VAH, m.VWAP, m.VAL)
	}
	return nil
}
