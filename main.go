package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"invest_intraday/internal/a_submodule/session"
	invariant "invest_intraday/internal/a_submodule/vah_gt_vwap_gt_val"
	"invest_intraday/internal/a_submodule/variable/VWAP_VA_5"
	"invest_intraday/internal/a_submodule/variable/VWAP_VA_res"
	"invest_intraday/internal/a_submodule/variable/VWAP_VA_today"
	"invest_intraday/internal/auth/alor"
	marketselection "invest_intraday/internal/market_prelaunch_selection"
	"invest_intraday/internal/order"
)

// Config описывает структуру файла config.json.
type Config struct {
	Auth struct {
		Alor struct {
			ClientID     string `json:"client_id"`
			RefreshToken string `json:"refresh_token"`
		} `json:"alor"`
	} `json:"auth"`
	Selection struct {
		SessionsToAnalyze int `json:"sessions_to_analyze"`
		SelectionLimit    int `json:"selection_limit"`
	} `json:"selection"`
	Markets []struct {
		Code              string    `json:"code"`
		VolatilityHistory []float64 `json:"volatility_history"`
	} `json:"markets"`
	OrderDefaults struct {
		Market     string  `json:"market"`
		Instrument string  `json:"instrument"`
		Quantity   int     `json:"quantity"`
		Price      float64 `json:"price"`
	} `json:"order_defaults"`
	Session struct {
		OfficialStart string `json:"official_start"`
		OfficialEnd   string `json:"official_end"`
	} `json:"session"`
	VWAPInputs struct {
		FivePeriod PriceVolumeInput `json:"five_period"`
		Today      PriceVolumeInput `json:"today"`
		Residual   PriceVolumeInput `json:"residual"`
	} `json:"vwap_inputs"`
}

type PriceVolumeInput struct {
	Prices  []float64 `json:"prices"`
	Volumes []float64 `json:"volumes"`
}

func main() {
	cfg, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("не удалось загрузить конфигурацию: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alorClient, err := alor.NewClient(alor.Credentials{
		ClientID:     cfg.Auth.Alor.ClientID,
		RefreshToken: cfg.Auth.Alor.RefreshToken,
	})
	if err != nil {
		log.Fatalf("ошибка инициализации клиента авторизации: %v", err)
	}

	token, err := alorClient.AccessToken(ctx)
	if err != nil {
		log.Fatalf("не удалось получить токен: %v", err)
	}
	log.Printf("Получен access token: %s", token)

	markets := make([]marketselection.MarketData, 0, len(cfg.Markets))
	for _, market := range cfg.Markets {
		markets = append(markets, marketselection.MarketData{
			Code:              market.Code,
			VolatilityHistory: market.VolatilityHistory,
		})
	}

	selectedMarkets, err := marketselection.SelectTopMarkets(
		markets,
		cfg.Selection.SessionsToAnalyze,
		cfg.Selection.SelectionLimit,
	)
	if err != nil {
		log.Fatalf("ошибка выбора рынков: %v", err)
	}

	log.Println("Выбранные рынки для старта:")
	for _, market := range selectedMarkets {
		log.Printf("- %s (волатильность %.4f)", market.Code, market.Volatility)
	}

	officialStart, officialEnd, err := parseSessionTimes(cfg.Session.OfficialStart, cfg.Session.OfficialEnd)
	if err != nil {
		log.Fatalf("ошибка парсинга времени сессии: %v", err)
	}

	boundaries, err := session.CalculateBoundaries(officialStart, officialEnd)
	if err != nil {
		log.Fatalf("ошибка расчёта границ сессии: %v", err)
	}
	log.Printf("Торговое окно: %s - %s", boundaries.TradingStart.Format(time.RFC3339), boundaries.TradingEnd.Format(time.RFC3339))

	vwap5, err := vwapva5.Calculate(cfg.VWAPInputs.FivePeriod.Prices, cfg.VWAPInputs.FivePeriod.Volumes)
	if err != nil {
		log.Fatalf("VWAP_VA_5: %v", err)
	}

	vwapToday, err := vwapvatoday.Calculate(cfg.VWAPInputs.Today.Prices, cfg.VWAPInputs.Today.Volumes)
	if err != nil {
		log.Fatalf("VWAP_VA_today: %v", err)
	}

	vwapRes, err := vwapvares.Calculate(cfg.VWAPInputs.Residual.Prices, cfg.VWAPInputs.Residual.Volumes)
	if err != nil {
		log.Fatalf("VWAP_VA_res: %v", err)
	}

	if err := invariant.Validate(vwapRes); err != nil {
		log.Fatalf("проверка инварианта провалена: %v", err)
	}

	log.Printf("VWAP_VA_5: VWAP=%.2f VAL=%.2f VAH=%.2f", vwap5.VWAP, vwap5.VAL, vwap5.VAH)
	log.Printf("VWAP_VA_today: VWAP=%.2f VAL=%.2f VAH=%.2f", vwapToday.VWAP, vwapToday.VAL, vwapToday.VAH)
	log.Printf("VWAP_VA_res: VWAP=%.2f VAL=%.2f VAH=%.2f", vwapRes.VWAP, vwapRes.VAL, vwapRes.VAH)

	marketForOrder := cfg.OrderDefaults.Market
	if marketForOrder == "" && len(selectedMarkets) > 0 {
		marketForOrder = selectedMarkets[0].Code
	}

	orderRequest, err := order.NewRequest(
		marketForOrder,
		cfg.OrderDefaults.Instrument,
		order.SideBuy,
		cfg.OrderDefaults.Quantity,
		cfg.OrderDefaults.Price,
	)
	if err != nil {
		log.Fatalf("ошибка подготовки заявки: %v", err)
	}

	log.Printf("Заявка к отправке: %s", orderRequest.Summary())
}

func loadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func parseSessionTimes(startRaw, endRaw string) (time.Time, time.Time, error) {
	start, err := time.Parse(time.RFC3339, startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	end, err := time.Parse(time.RFC3339, endRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start, end, nil
}
