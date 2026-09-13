// binance-trade-bot — Go backend entrypoint.
//
// Env:
//
//	PORT               (default 8080; Render sets this)
//	DATA_DIR           (default ./data — JSON store)
//	TRADING_MODE, TRADING_SYMBOLS, TIMEFRAME, STRATEGY, ... (seed config)
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"binance-trade-bot/internal/api"
	"binance-trade-bot/internal/config"
	"binance-trade-bot/internal/engine"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	abs, _ := filepath.Abs(dataDir)
	config.Init(abs)
	log.Printf("data dir: %s", abs)

	// seed config from env on first boot
	cfg := config.LoadConfig()
	_ = config.SaveConfig(cfg)
	cfg = config.LoadConfig()
	log.Printf("mode=%s strategy=%s symbols=%s timeframe=%s", cfg.TradingMode, cfg.Strategy, cfg.TradingSymbols, cfg.Timeframe)

	// start the trading loop
	eng := engine.Get()
	eng.Start()
	log.Printf("engine started (poll=%ds)", cfg.PollInterval)

	mux := api.Mux()
	log.Printf("listening on :%s", port)
	if err := http.ListenAndServe(":"+port, api.CORS(mux)); err != nil {
		log.Fatal(err)
	}
}
