package indicators

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"invest_intraday/internal/a_submodule/moex"
	"invest_intraday/internal/a_technical/db"
	"invest_intraday/models"
)

type sessionFetcher struct {
	tickerRepo *db.TickerInfoRepository
	issClient  *moex.ISSClient

	boardCache map[string]moex.BoardMetadata
	cacheMu    sync.Mutex
}

func newSessionFetcher(tickerRepo *db.TickerInfoRepository, issClient *moex.ISSClient) *sessionFetcher {
	if tickerRepo == nil || issClient == nil {
		return nil
	}
	return &sessionFetcher{
		tickerRepo: tickerRepo,
		issClient:  issClient,
		boardCache: make(map[string]moex.BoardMetadata),
	}
}

func (f *sessionFetcher) mainSessionTrades(ctx context.Context, tickerInfoID int64, sessionDate time.Time) (models.TickerInfo, []moex.Trade, error) {
	if f == nil {
		return models.TickerInfo{}, nil, fmt.Errorf("session fetcher is not configured")
	}

	info, err := f.tickerRepo.GetByID(ctx, tickerInfoID)
	if err != nil {
		return models.TickerInfo{}, nil, fmt.Errorf("load ticker info: %w", err)
	}

	board, err := f.boardMetadata(ctx, info.BoardID)
	if err != nil {
		return models.TickerInfo{}, nil, fmt.Errorf("load board metadata: %w", err)
	}

	trades, err := f.issClient.Trades(ctx, board, info.BoardID, info.SecID, sessionDate)
	if err != nil {
		return models.TickerInfo{}, nil, fmt.Errorf("load trades: %w", err)
	}

	mainTrades, err := filterMainSession(trades)
	if err != nil {
		return models.TickerInfo{}, nil, err
	}

	return info, mainTrades, nil
}

func (f *sessionFetcher) boardMetadata(ctx context.Context, boardID string) (moex.BoardMetadata, error) {
	boardID = strings.ToUpper(boardID)

	f.cacheMu.Lock()
	board, ok := f.boardCache[boardID]
	f.cacheMu.Unlock()
	if ok {
		return board, nil
	}

	board, err := f.issClient.BoardMetadata(ctx, boardID)
	if err != nil {
		return moex.BoardMetadata{}, err
	}

	f.cacheMu.Lock()
	f.boardCache[boardID] = board
	f.cacheMu.Unlock()

	return board, nil
}
