package engine

import (
	"testing"
	"time"

	"binance-trade-bot/internal/config"
	"binance-trade-bot/internal/exchange"
)

// TestCooldownBlocksEntry: symbol trong cooldown không được entry lại.
func TestCooldownBlocksEntry(t *testing.T) {
	tmpDir(t)
	e := &Engine{
		cooldowns: map[string]time.Time{},
	}
	if e.inCooldown("BTCUSDT") {
		t.Fatal("empty cooldown should not block")
	}
	e.setCooldown("BTCUSDT", 10)
	if !e.inCooldown("BTCUSDT") {
		t.Fatal("BTCUSDT should be cooling down")
	}
	if e.inCooldown("ETHUSDT") {
		t.Fatal("ETHUSDT should not be affected")
	}
}

// TestCooldownExpires: cooldown quá hạn → không chặn nữa.
func TestCooldownExpires(t *testing.T) {
	tmpDir(t)
	e := &Engine{cooldowns: map[string]time.Time{}}
	e.cooldowns["BTCUSDT"] = time.Now().Add(-1 * time.Minute) // đã quá hạn
	if e.inCooldown("BTCUSDT") {
		t.Fatal("expired cooldown should not block")
	}
	// và phải bị dọn khỏi map
	e.cooldownsMu.Lock()
	n := len(e.cooldowns)
	e.cooldownsMu.Unlock()
	if n != 0 {
		t.Fatalf("expired cooldown should be cleaned, still %d", n)
	}
}

// TestCooldownZeroDisabled: minutes=0 → không set gì.
func TestCooldownZeroDisabled(t *testing.T) {
	tmpDir(t)
	e := &Engine{cooldowns: map[string]time.Time{}}
	e.setCooldown("BTCUSDT", 0)
	if e.inCooldown("BTCUSDT") {
		t.Fatal("cooldown 0 minutes must be a no-op")
	}
}

// TestLotRoundQty: làm tròn theo stepSize (floor), giữ maxQty.
func TestLotRoundQty(t *testing.T) {
	lf := exchange.LotFilter{StepSize: 0.001, MinQty: 0.001, MaxQty: 100}
	cases := []struct {
		in, want float64
	}{
		{0.123456, 0.123},
		{1.999999, 1.999},
		{0.0005, 0.0},
		{100.5, 100.0},
	}
	for _, c := range cases {
		got := lf.RoundQty(c.in)
		if got != c.want {
			t.Fatalf("RoundQty(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestLotRoundQtyNoStep: stepSize=0 → floor 8 chữ số thập phân.
func TestLotRoundQtyNoStep(t *testing.T) {
	lf := exchange.LotFilter{}
	got := lf.RoundQty(0.123456789)
	if got != 0.12345678 {
		t.Fatalf("RoundQty no step = %v", got)
	}
}

// TestSellQuoteValue: ước tính net sau phí 0.1%.
func TestSellQuoteValue(t *testing.T) {
	// 100 coin × $50 = $5000 gross → net = 5000 × 0.999 = 4995
	got := exchange.SellQuoteValue(100, 50)
	if got < 4994.99 || got > 4995.01 {
		t.Fatalf("SellQuoteValue = %v, want ~4995", got)
	}
}

// TestPartialSellPaper: đóng 50% — wallet cập nhật, trade giữ lại nửa.
// Paper mode fetch giá thật từ testnet → chạy online. Nếu offline thì skip.
func TestPartialSellPaper(t *testing.T) {
	tmpDir(t)
	cfg := baseCfg()
	e := &Engine{}

	entry := 100.0
	e.createTrade(cfg, "paper", "SOLUSDT", 2.0, entry, 200, 0.2, "adaptive_grid", "test")

	qty, _, err := e.PartialSell(1, 50)
	if err != nil {
		t.Fatalf("partial sell failed: %v", err)
	}
	if qty != 1.0 {
		t.Fatalf("sold qty = %v, want 1.0", qty)
	}

	db := config.LoadDB()
	tr := db.Trades[0]
	if tr.Status != "open" {
		t.Fatal("trade should stay open after partial close")
	}
	if tr.Qty != 1.0 {
		t.Fatalf("remaining qty = %v, want 1.0", tr.Qty)
	}
	if tr.Stake != 100 {
		t.Fatalf("remaining stake = %v, want 100", tr.Stake)
	}
	// order log có dòng partial_sell
	found := false
	for _, o := range db.Orders {
		if o.Action == "partial_sell" {
			found = true
		}
	}
	if !found {
		t.Fatal("no partial_sell order log row")
	}
}

// TestPartialSellInvalidPct: pct ngoài 1–100 bị từ chối.
func TestPartialSellInvalidPct(t *testing.T) {
	tmpDir(t)
	e := &Engine{}
	if _, _, err := e.PartialSell(1, 0); err == nil {
		t.Fatal("pct=0 must be rejected")
	}
	if _, _, err := e.PartialSell(1, 150); err == nil {
		t.Fatal("pct=150 must be rejected")
	}
}

// TestDailyLossLimitPausesBot: thua đủ limit trong ngày → BotRunning=false.
func TestDailyLossLimitPausesBot(t *testing.T) {
	tmpDir(t)
	cfg := baseCfg()
	cfg.DailyLossLimitPct = 5
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	e := &Engine{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_ = config.Mutate(func(c *config.Collection) {
		c.Equity = append(c.Equity,
			config.EquityPoint{ID: 1, TS: now, Equity: 1000},
			config.EquityPoint{ID: 2, TS: now, Equity: 940}, // −6% trong ngày
		)
	})
	e.dailyLossCheck(cfg)
	got := config.LoadConfig()
	if got.BotRunning {
		t.Fatal("bot should be paused after breaching daily loss limit")
	}
	// log ERROR phải được ghi
	db := config.LoadDB()
	found := false
	for _, l := range db.Logs {
		if l.Level == "ERROR" && l.Module == "risk" {
			found = true
		}
	}
	if !found {
		t.Fatal("daily loss pause should log ERROR")
	}
}

// TestDailyLossLimitNotBreached: thua ít hơn limit → không pause.
func TestDailyLossLimitNotBreached(t *testing.T) {
	tmpDir(t)
	cfg := baseCfg()
	cfg.DailyLossLimitPct = 5
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	e := &Engine{}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_ = config.Mutate(func(c *config.Collection) {
		c.Equity = append(c.Equity,
			config.EquityPoint{ID: 1, TS: now, Equity: 1000},
			config.EquityPoint{ID: 2, TS: now, Equity: 990}, // −1% < 5%
		)
	})
	e.dailyLossCheck(cfg)
	if !config.LoadConfig().BotRunning {
		t.Fatal("bot should stay running below the daily loss limit")
	}
}

// TestDailyLossLimitNoBaseline: chưa có equity hôm nay → không pause.
func TestDailyLossLimitNoBaseline(t *testing.T) {
	tmpDir(t)
	cfg := baseCfg()
	cfg.DailyLossLimitPct = 5
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	e := &Engine{}
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format(time.RFC3339Nano)
	_ = config.Mutate(func(c *config.Collection) {
		c.Equity = append(c.Equity, config.EquityPoint{ID: 1, TS: yesterday, Equity: 1000})
	})
	e.dailyLossCheck(cfg)
	if !config.LoadConfig().BotRunning {
		t.Fatal("no today baseline → must not pause")
	}
}
