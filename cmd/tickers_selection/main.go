package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"invest_intraday/internal/a_technical/config"
	"invest_intraday/internal/tickers_selection"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("нужно передать путь до config.json")
	}

	cfgPath := os.Args[1]

	cfg, err := config.FromFile(cfgPath)
	if err != nil {
		log.Fatalf("загрузка конфигурации: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelTimeout()

	service, err := tickers_selection.NewService(ctx, cfg)
	if err != nil {
		log.Fatalf("инициализация сервиса: %v", err)
	}
	defer service.Close()

	result, err := service.Run(ctx)
	if err != nil {
		log.Fatalf("ошибка выполнения tickers_selection: %v", err)
	}

	log.Printf("tickers_selection: обновлено записей: существующих=%d, созданных=%d",
		result.FillingStats.Existing, result.FillingStats.Created)

	if len(result.Selected) == 0 {
		log.Println("tickers_selection: подходящих тикеров не найдено")
		return
	}

	fmt.Println("Лучшие тикеры по итоговым баллам:")
	for i, item := range result.Selected {
		fmt.Printf("%d. %s — режим: %s, итоговый балл: %.2f (Range: %.2f, Trend: %.2f, TrendScore: %.2f, ΔVWAP%%: %.2f, Overlap%%: %.2f, Breakout: %t)\n",
			i+1,
			item.Ticker,
			item.Regime,
			item.FinalScore,
			item.MeanReversionScore,
			item.MomentumScore,
			item.TrendScore,
			item.DeltaVWAPPct,
			item.OverlapPercent,
			item.Breakout,
		)
	}
}
