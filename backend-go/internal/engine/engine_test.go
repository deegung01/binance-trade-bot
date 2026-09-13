package engine

import (
	"encoding/json"
	"os"
	"testing"

	"binance-trade-bot/internal/config"
	"binance-trade-bot/internal/exchange"
	"binance-trade-bot/internal/strategy"
)

func tmpDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	config.Init(d)
	return d
}

func baseCfg() config.Config {
	return config.Config{
		TradingMode: "paper", StartBalance: 10000, StakeAmount: 100,
		StopLossPct: 2, TakeProfitPct: 4, MaxOpenTrades: 3,
		TrailingStop: true, TrailingStopPct: 1, GridLevels: 3,
		Strategy: "adaptive_grid", BotRunning: true,
	}
}

func mkCandle(close float64) exchange.Candle {
	return exchange.Candle{Open: close, High: close, Low: close, Close: close, OpenTime: 1700000000000}
}

// TestTrailingStopRatchet: SL chỉ nâng lên, thoát bằng trailing_stop.
func TestTrailingStopRatchet(t *testing.T) {
	tmpDir(t)
	cfg := baseCfg()
	st := config.State{Cash: 10000, Positions: map[string]config.Position{}, InitBalance: 10000}
	e := &Engine{}

	e.createTrade(cfg, "paper", "ETHUSDT", 0.1, 2500, 250, 0.25, "manual", "test")
	db := config.LoadDB()
	tr := db.Trades[0]
	origSL := tr.StopLoss

	// giá +3% → kích hoạt trailing, SL = high*0.99
	p1 := tr.EntryPrice * 1.03
	e.manageTrade(cfg, "paper", &st, &tr, p1, []exchange.Candle{mkCandle(p1)})
	db = config.LoadDB()
	tr = db.Trades[0]
	if tr.StopLoss <= origSL {
		t.Fatalf("trailing SL not raised: %v <= %v", tr.StopLoss, origSL)
	}
	want := tr.Meta.TrailHigh * 0.99
	if tr.StopLoss != want {
		t.Fatalf("SL %v != trail_high*0.99 %v", tr.StopLoss, want)
	}
	if !tr.Meta.TrailActivated {
		t.Fatal("trail not activated at +3%")
	}

	// giá +5% → SL ratchet tiếp
	slBefore := tr.StopLoss
	p2 := tr.EntryPrice * 1.05
	e.manageTrade(cfg, "paper", &st, &tr, p2, []exchange.Candle{mkCandle(p2)})
	db = config.LoadDB()
	sl2 := db.Trades[0].StopLoss
	if sl2 <= slBefore {
		t.Fatalf("SL should ratchet up: %v <= %v", sl2, slBefore)
	}

	// giá tụt về SL trailing → thoát trailing_stop với lãi
	db = config.LoadDB()
	tr = db.Trades[0]
	p3 := tr.StopLoss * 0.999
	e.manageTrade(cfg, "paper", &st, &tr, p3, []exchange.Candle{mkCandle(p3)})
	db = config.LoadDB()
	final := db.Trades[0]
	if final.Status != "closed" {
		t.Fatal("trade should be closed")
	}
	if final.ExitReason != "trailing_stop" {
		t.Fatalf("exit reason = %v, want trailing_stop", final.ExitReason)
	}
	if final.PnL <= 0 {
		t.Fatalf("trailing exit should lock profit, got %v", final.PnL)
	}
}

// TestGridDCA: adaptive grid add khi giá tụt 1 spacing + rebase avg.
func TestGridDCA(t *testing.T) {
	tmpDir(t)
	cfg := baseCfg()
	st := config.State{Cash: 10000, Positions: map[string]config.Position{}, InitBalance: 10000}
	e := &Engine{}

	entry := 100.0
	e.createTrade(cfg, "paper", "SOLUSDT", 0.99, entry, 100, 0.1, "adaptive_grid", "test")
	// seed position cho wallet
	st.Positions["SOLUSDT"] = config.Position{Qty: 0.99, Cost: entry}

	db := config.LoadDB()
	tr := db.Trades[0]

	// spacing tính từ ATR — dùng spacing thực tế của strategy
	// mô phỏng giá tụt đủ 1 spacing: dựng 20 nến phẳng rồi tụt
	candles := []exchange.Candle{}
	for i := 0; i < 20; i++ {
		candles = append(candles, exchange.Candle{Open: entry, High: entry * 1.001, Low: entry * 0.999, Close: entry, OpenTime: int64(i) * 300000})
	}
	ag := strategy.Get("adaptive_grid")
	// spacing trên giá phẳng ~0.5% (min clamp) → trigger = entry*0.995
	trigger := entry * (1 - 0.005)
	candles = append(candles, mkCandle(trigger))

	extra := ag.AdjustSignal(candles, &tr, cfg)
	if extra != 100 {
		t.Fatalf("grid add should trigger with 100 USDT, got %v", extra)
	}
	e.gridAdd(cfg, "paper", &st, &tr, extra, trigger)

	db = config.LoadDB()
	after := db.Trades[0]
	if after.Meta.GridCount != 1 {
		t.Fatalf("grid_count = %v, want 1", after.Meta.GridCount)
	}
	if after.EntryPrice >= entry {
		t.Fatalf("avg should drop after DCA: %v >= %v", after.EntryPrice, entry)
	}
	if after.Qty <= 0.99 {
		t.Fatalf("qty should grow: %v", after.Qty)
	}
	// wallet position cũng phải cộng dồn
	if st.Positions["SOLUSDT"].Qty <= 0.99 {
		t.Fatalf("wallet position not updated: %v", st.Positions["SOLUSDT"])
	}
}

// TestConfigPersist: save/load qua JSON file.
func TestConfigPersist(t *testing.T) {
	tmpDir(t)
	cfg := baseCfg()
	cfg.Strategy = "macd"
	cfg.TrailingStop = true
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got := config.LoadConfig()
	if got.Strategy != "macd" || !got.TrailingStop {
		t.Fatalf("config not persisted: %+v", got)
	}
}

// TestStateJSON: wallet round-trip.
func TestStateJSON(t *testing.T) {
	tmpDir(t)
	st := config.State{Cash: 9500, Positions: map[string]config.Position{"BTCUSDT": {Qty: 0.01, Cost: 76000}}, InitBalance: 10000}
	if err := config.SaveState(st); err != nil {
		t.Fatal(err)
	}
	got := config.LoadState(baseCfg())
	if got.Cash != 9500 || got.Positions["BTCUSDT"].Qty != 0.01 {
		t.Fatalf("state round-trip failed: %+v", got)
	}
	b, _ := json.Marshal(st)
	if len(b) == 0 {
		t.Fatal("empty json")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
