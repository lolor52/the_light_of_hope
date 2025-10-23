package indicators

import (
	"context"
	"fmt"
	"strings"
	"time"

	"invest_intraday/internal/a_submodule/alor"
	"invest_intraday/internal/a_technical/db"
	"invest_intraday/models"
)

type sessionFetcher struct {
	tickerRepo *db.TickerInfoRepository
	alorClient *alor.Client
}

func newSessionFetcher(tickerRepo *db.TickerInfoRepository, alorClient *alor.Client) *sessionFetcher {
	if tickerRepo == nil || alorClient == nil {
		return nil
	}
	return &sessionFetcher{
		tickerRepo: tickerRepo,
		alorClient: alorClient,
	}
}

func (f *sessionFetcher) mainSessionTrades(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (models.TickerInfo, []alor.Trade, error) {
	if f == nil {
		return models.TickerInfo{}, nil, fmt.Errorf("session fetcher is not configured")
	}

	info, err := f.tickerRepo.GetByID(ctx, tickerInfoID)
	if err != nil {
		return models.TickerInfo{}, nil, fmt.Errorf("load ticker info: %w", err)
	}

	instrument := f.instrumentFromInfo(info)
	trades, err := f.alorClient.Trades(ctx, instrument, sessionDate)
	if err != nil {
		return models.TickerInfo{}, nil, fmt.Errorf("load trades: %w", err)
	}

	mainTrades, err := filterMainSession(trades)
	if err != nil {
		return models.TickerInfo{}, nil, err
	}

	return info, mainTrades, nil
}

func (f *sessionFetcher) instrumentFromInfo(info models.TickerInfo) alor.Instrument {
	exchange := alor.ExchangeMOEX
	board := strings.ToUpper(strings.TrimSpace(info.BoardID))
	if strings.HasPrefix(board, "SPB") {
		exchange = alor.ExchangeSPB
	}

	symbol := strings.TrimSpace(info.SecID)
	if symbol == "" {
		symbol = strings.TrimSpace(info.TickerName)
	}
	symbol = strings.ToUpper(symbol)

	return alor.Instrument{
		Exchange: exchange,
		Board:    board,
		Symbol:   symbol,
	}
}
