package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"invest_intraday/internal/a_submodule/tickers_filling"
	"invest_intraday/internal/a_technical/config"
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

	service, err := tickers_filling.NewService(ctx, cfg)
	if err != nil {
		log.Fatalf("инициализация сервиса: %v", err)
	}
	defer service.Close()

	if _, err := service.Run(ctx); err != nil {
		log.Fatalf("ошибка выполнения tickers_filling: %v", err)
	}
}
