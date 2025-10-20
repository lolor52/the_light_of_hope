package marketprelaunchselection

import (
	"errors"
	"math"
	"sort"
)

// MarketData описывает рыночные данные, используемые для расчёта волатильности.
type MarketData struct {
	Code              string
	VolatilityHistory []float64
}

// MarketVolatility содержит рассчитанную волатильность рынка.
type MarketVolatility struct {
	Code       string
	Volatility float64
}

// SelectTopMarkets рассчитывает волатильность и выбирает X наиболее волатильных рынков.
func SelectTopMarkets(markets []MarketData, sessionsToAnalyze, selectionLimit int) ([]MarketVolatility, error) {
	if sessionsToAnalyze <= 0 {
		return nil, errors.New("количество сессий должно быть положительным")
	}
	if selectionLimit <= 0 {
		return nil, errors.New("количество рынков для выбора должно быть положительным")
	}
	if len(markets) == 0 {
		return nil, errors.New("список рынков пуст")
	}

	calculated := make([]MarketVolatility, 0, len(markets))
	for _, market := range markets {
		if len(market.VolatilityHistory) < sessionsToAnalyze {
			return nil, errors.New("недостаточно данных для рынка " + market.Code)
		}

		sample := market.VolatilityHistory[len(market.VolatilityHistory)-sessionsToAnalyze:]
		volatility := stdDev(sample)
		calculated = append(calculated, MarketVolatility{Code: market.Code, Volatility: volatility})
	}

	sort.Slice(calculated, func(i, j int) bool {
		if calculated[i].Volatility == calculated[j].Volatility {
			return calculated[i].Code < calculated[j].Code
		}
		return calculated[i].Volatility > calculated[j].Volatility
	})

	if selectionLimit > len(calculated) {
		selectionLimit = len(calculated)
	}

	return calculated[:selectionLimit], nil
}

func stdDev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	mean := mean(values)
	var sum float64
	for _, value := range values {
		diff := value - mean
		sum += diff * diff
	}
	variance := sum / float64(len(values))
	return math.Sqrt(variance)
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}
