package order

import (
	"errors"
	"fmt"
)

// Side описывает направление сделки.
type Side string

const (
	// SideBuy представляет заявку на покупку.
	SideBuy Side = "buy"
	// SideSell представляет заявку на продажу.
	SideSell Side = "sell"
)

// Request содержит параметры выставления заявки.
type Request struct {
	Market     string
	Instrument string
	Side       Side
	Quantity   int
	Price      float64
}

// NewRequest создаёт заявку и проверяет входные данные.
func NewRequest(market, instrument string, side Side, quantity int, price float64) (Request, error) {
	if market == "" {
		return Request{}, errors.New("market не может быть пустым")
	}
	if instrument == "" {
		return Request{}, errors.New("instrument не может быть пустым")
	}
	if side != SideBuy && side != SideSell {
		return Request{}, errors.New("неподдерживаемая сторона заявки")
	}
	if quantity <= 0 {
		return Request{}, errors.New("количество должно быть положительным")
	}
	if price <= 0 {
		return Request{}, errors.New("цена должна быть положительной")
	}

	return Request{
		Market:     market,
		Instrument: instrument,
		Side:       side,
		Quantity:   quantity,
		Price:      price,
	}, nil
}

// Summary возвращает короткое текстовое описание заявки.
func (r Request) Summary() string {
	return fmt.Sprintf("%s %d %s по цене %.2f на рынке %s", r.Side, r.Quantity, r.Instrument, r.Price, r.Market)
}
