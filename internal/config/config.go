// Package config — runtime bot configuration.
// Single source of truth: the config store (JSON file on disk, DB-free).
// Seed values come from environment variables (Render env vars).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// Config is the whole bot configuration, editable from the dashboard.
type Config struct {
	TradingMode      string  `json:"trading_mode"`       // paper | live
	PaperDataSource  string  `json:"paper_data_source"`   // testnet | mainnet
	StartBalance     float64 `json:"start_balance"`
	TradingSymbols   string  `json:"trading_symbols"`
	Timeframe        string  `json:"timeframe"`
	Strategy         string  `json:"strategy"`
	StakeMode        string  `json:"stake_mode"` // fixed | percent
	StakeAmount      float64 `json:"stake_amount"`
	StakePercent     float64 `json:"stake_percent"`
	StopLossPct      float64 `json:"stop_loss_pct"`
	TakeProfitPct    float64 `json:"take_profit_pct"`
	MaxOpenTrades    int     `json:"max_open_trades"`
	PollInterval     int     `json:"poll_interval"`
	TrailingStop     bool    `json:"trailing_stop"`
	TrailingStopPct  float64 `json:"trailing_stop_pct"`
	GridLevels       int     `json:"grid_levels"`
	BotRunning       bool    `json:"bot_running"`
	ResetRequested    bool    `json:"reset_requested"`

	// Risk guards
	CooldownMinutes  int     `json:"cooldown_minutes"`  // sau stop_loss, không entry lại symbol đó trong N phút (0 = off)
	DailyLossLimitPct float64 `json:"daily_loss_limit_pct"` // mất ≥ X% equity trong ngày → pause bot (0 = off)

	// credentials (never sent to the dashboard)
	BinanceAPIKey    string  `json:"-"`
	BinanceAPISecret string  `json:"-"`
}

// State is the paper wallet.
type State struct {
	Cash         float64            `json:"cash"`
	Positions    map[string]Position `json:"positions"`
	TradeSeq     int64              `json:"trade_seq"`
	InitBalance  float64            `json:"init_balance"`
	StartedAt    string             `json:"started_at"`
	LiveBaseline float64            `json:"live_baseline,omitempty"` // equity testnet khi vào live mode lần đầu
}

// Position is an open paper position.
type Position struct {
	Qty  float64 `json:"qty"`
	Cost float64 `json:"cost"` // weighted average entry price
}

// Trade mirrors the SQL schema from the Python version.
type Trade struct {
	ID           int64    `json:"id"`
	Symbol       string   `json:"symbol"`
	Side         string   `json:"side"` // buy
	Status       string   `json:"status"` // open | closed
	Mode         string   `json:"mode"` // paper | live
	Qty          float64  `json:"qty"`
	EntryPrice   float64  `json:"entry_price"`
	ExitPrice    *float64 `json:"exit_price"`
	StopLoss     float64  `json:"stop_loss"`
	TakeProfit   float64  `json:"take_profit"`
	Stake        float64  `json:"stake"`
	PnL          float64  `json:"pnl"`
	PnLPct       float64  `json:"pnl_pct"`
	Fee          float64  `json:"fee"`
	Strategy     string   `json:"strategy"`
	SignalReason string   `json:"signal_reason"`
	ExitReason   string   `json:"exit_reason"`
	OpenedAt     string   `json:"opened_at"`
	ClosedAt     *string  `json:"closed_at"`
	Meta         *TradeMeta `json:"meta"`
}

// TradeMeta holds grid/trailing/regime runtime info.
type TradeMeta struct {
	GridCount      int     `json:"grid_count"`
	InitialStake   float64 `json:"initial_stake"`
	LastAddPrice   float64 `json:"last_add_price"`
	TrailActivated bool    `json:"trail_activated"`
	TrailHigh      float64 `json:"trail_high"`
	EntryRegime    string  `json:"entry_regime,omitempty"`
	Converted      bool    `json:"converted,omitempty"`
	VolTrailPct    float64 `json:"vol_trail_pct,omitempty"`
}

// OrderLog is an entry in the order activity feed.
type OrderLog struct {
	ID              int64   `json:"id"`
	TradeID         *int64  `json:"trade_id"`
	Symbol          string  `json:"symbol"`
	Action          string  `json:"action"` // buy | sell | grid_add | error | info
	Mode            string  `json:"mode"`
	Qty             *float64 `json:"qty"`
	Price           *float64 `json:"price"`
	Status          string  `json:"status"`
	Detail          string  `json:"detail"`
	CreatedAt       string  `json:"created_at"`
}

// LogEntry is an engine log line.
type LogEntry struct {
	ID        int64  `json:"id"`
	Level     string `json:"level"` // INFO | WARN | ERROR
	Module    string `json:"module"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

// EquityPoint is a snapshot of the paper wallet value.
type EquityPoint struct {
	ID             int64   `json:"id"`
	TS             string  `json:"ts"`
	Equity         float64 `json:"equity"`
	Cash           float64 `json:"cash"`
	PositionsValue float64 `json:"positions_value"`
	Mode           string  `json:"mode"`
}

// ---------------------------------------------------------------------------
// Store: JSON-file-backed persistence (works on Render ephemeral disk).
// ---------------------------------------------------------------------------

var (
	mu       sync.Mutex
	dataDir  string
	seedEnv  Config
)

func Init(dir string) {
	dataDir = dir
	_ = os.MkdirAll(dir, 0o755)
}

func defaultConfig() Config {
	mode := envStr("TRADING_MODE", "paper")
	// Chỉ nhận paper|live — mọi giá trị khác (kể cả tên biến bị dán nhầm
	// trên Render) rơi về paper để không bao giờ đặt lệnh thật ngoài ý muốn.
	if mode != "live" {
		mode = "paper"
	}
	return Config{
		TradingMode:     mode,
		PaperDataSource: envStr("PAPER_DATA_SOURCE", "testnet"),
		StartBalance:    envFloat("START_BALANCE", 10000),
		TradingSymbols:  envStr("TRADING_SYMBOLS", "BTCUSDT,ETHUSDT,SOLUSDT,BNBUSDT"),
		Timeframe:       envStr("TIMEFRAME", "5m"),
		Strategy:        envStr("STRATEGY", "sma_cross"),
		StakeMode:       envStr("STAKE_MODE", "fixed"),
		StakeAmount:     envFloat("STAKE_AMOUNT", 100),
		StakePercent:    envFloat("STAKE_PERCENT", 10),
		StopLossPct:     envFloat("STOP_LOSS_PCT", 2),
		TakeProfitPct:   envFloat("TAKE_PROFIT_PCT", 4),
		MaxOpenTrades:   int(envFloat("MAX_OPEN_TRADES", 3)),
		PollInterval:    int(envFloat("POLL_INTERVAL", 15)),
		TrailingStop:    envStr("TRAILING_STOP", "false") == "true",
		TrailingStopPct: envFloat("TRAILING_STOP_PCT", 1),
		GridLevels:      int(envFloat("GRID_LEVELS", 4)),
		CooldownMinutes: int(envFloat("COOLDOWN_MINUTES", 0)),
		DailyLossLimitPct: envFloat("DAILY_LOSS_LIMIT_PCT", 0),
		BotRunning:      true,
		BinanceAPIKey:    os.Getenv("BINANCE_API_KEY"),
		BinanceAPISecret: os.Getenv("BINANCE_API_SECRET"),
	}
}

func envStr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// LoadConfig reads config from disk, seeding from env on first start.
func LoadConfig() Config {
	mu.Lock()
	defer mu.Unlock()
	return loadConfigLocked()
}

func loadConfigLocked() Config {
	cfg := defaultConfig()
	b, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(b, &cfg)
		// credentials always from env (never persisted)
		cfg.BinanceAPIKey = os.Getenv("BINANCE_API_KEY")
		cfg.BinanceAPISecret = os.Getenv("BINANCE_API_SECRET")
	}
	return cfg
}

// SaveConfig persists the config.
func SaveConfig(cfg Config) error {
	mu.Lock()
	defer mu.Unlock()
	cfg.BinanceAPIKey = ""
	cfg.BinanceAPISecret = ""
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), b, 0o644)
}

func configPath() string { return filepath.Join(dataDir, "config.json") }

// LoadState / SaveState — paper wallet.
func LoadState(seed Config) State {
	mu.Lock()
	defer mu.Unlock()
	st := State{Cash: seed.StartBalance, Positions: map[string]Position{}, InitBalance: seed.StartBalance}
	b, err := os.ReadFile(statePath())
	if err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Positions == nil {
		st.Positions = map[string]Position{}
	}
	return st
}

func SaveState(st State) error {
	mu.Lock()
	defer mu.Unlock()
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), b, 0o644)
}

func statePath() string { return filepath.Join(dataDir, "state.json") }

// ---------------------------------------------------------------------------
// Collections (trades / logs / activity / equity) — JSON files with IDs.
// ---------------------------------------------------------------------------

type collection struct {
	Trades    []Trade       `json:"trades"`
	Orders    []OrderLog    `json:"orders"`
	Logs      []LogEntry    `json:"logs"`
	Equity    []EquityPoint `json:"equity"`
}

// Collection is the persisted trade/order/log/equity store.
type Collection = collection

func dbPath() string { return filepath.Join(dataDir, "db.json") }

func LoadDB() collection {
	mu.Lock()
	defer mu.Unlock()
	var c collection
	b, err := os.ReadFile(dbPath())
	if err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func SaveDB(c collection) error {
	mu.Lock()
	defer mu.Lock()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dbPath(), b, 0o644)
}

// Mutate runs fn under lock and persists the collection.
func Mutate(fn func(c *collection)) error {
	mu.Lock()
	c := collection{}
	if b, err := os.ReadFile(dbPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	fn(&c)
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		mu.Unlock()
		return err
	}
	err = os.WriteFile(dbPath(), b, 0o644)
	mu.Unlock()
	return err
}

// Reset wipes everything (trades/logs/equity) and rebuilds the wallet.
// Live-mode accounts (Binance testnet API) are NEVER paper-reset: the
// testnet wallet is server-side at Binance, so we only clear local history
// and keep the wallet fields untouched (they mirror the remote account).
func Reset(seed Config) {
	mu.Lock()
	defer mu.Unlock()
	prev := State{}
	if b, err := os.ReadFile(statePath()); err == nil {
		_ = json.Unmarshal(b, &prev)
	}
	_ = os.WriteFile(dbPath(), []byte("{}"), 0o644)
	st := State{Cash: seed.StartBalance, Positions: map[string]Position{}, InitBalance: seed.StartBalance}
	if seed.TradingMode == "live" {
		st.Cash = prev.Cash
		st.Positions = prev.Positions
		st.InitBalance = prev.InitBalance
		st.LiveBaseline = prev.LiveBaseline
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(statePath(), b, 0o644)
}
