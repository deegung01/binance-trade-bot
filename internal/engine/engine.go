// Package engine — the trading loop (paper/live, trailing, grid DCA).
package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"binance-trade-bot/internal/config"
	"binance-trade-bot/internal/exchange"
	"binance-trade-bot/internal/regime"
	"binance-trade-bot/internal/strategy"
)

const (
	tradeFeeRate  = exchange.FeeRate
	statusNone    = ""
	reasonSignal  = "signal"
)

// Engine runs the trading cycle on a ticker.
type Engine struct {
	mu         sync.Mutex
	lastCycle  map[string]any
	stopCh     chan struct{}
	once       sync.Once
	regimes    map[string]regime.Snapshot // symbol → latest regime snapshot
	regimesMu  sync.Mutex
	cooldowns  map[string]time.Time // symbol → thời điểm được phép entry lại
	cooldownsMu sync.Mutex
	converts  map[string]bool // convert job id → done
	convertsMu sync.Mutex
}

var defaultEngine *Engine

// Get returns the singleton engine.
func Get() *Engine {
	if defaultEngine == nil {
		defaultEngine = &Engine{
			lastCycle: map[string]any{},
			regimes:   map[string]regime.Snapshot{},
			cooldowns: map[string]time.Time{},
			converts: map[string]bool{},
		}
	}
	return defaultEngine
}

// Cooldowns returns a copy of the cooldown map (symbol → allowed-after time).
func (e *Engine) Cooldowns() map[string]time.Time {
	e.cooldownsMu.Lock()
	defer e.cooldownsMu.Unlock()
	out := make(map[string]time.Time, len(e.cooldowns))
	for k, v := range e.cooldowns {
		out[k] = v
	}
	return out
}

// setCooldown ghi nhận symbol vừa bị stop_loss → chờ N phút trước khi entry lại.
func (e *Engine) setCooldown(symbol string, minutes int) {
	if minutes <= 0 {
		return
	}
	e.cooldownsMu.Lock()
	e.cooldowns[symbol] = time.Now().Add(time.Duration(minutes) * time.Minute)
	e.cooldownsMu.Unlock()
}

// inCooldown checks whether a symbol is still cooling down.
func (e *Engine) inCooldown(symbol string) bool {
	e.cooldownsMu.Lock()
	defer e.cooldownsMu.Unlock()
	until, ok := e.cooldowns[symbol]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(e.cooldowns, symbol)
		return false
	}
	return true
}

// Regimes returns a copy of the latest per-symbol regime snapshots.
func (e *Engine) Regimes() map[string]regime.Snapshot {
	e.regimesMu.Lock()
	defer e.regimesMu.Unlock()
	out := make(map[string]regime.Snapshot, len(e.regimes))
	for k, v := range e.regimes {
		out[k] = v
	}
	return out
}

func (e *Engine) setRegime(sym string, sn regime.Snapshot) {
	e.regimesMu.Lock()
	e.regimes[sym] = sn
	e.regimesMu.Unlock()
}

// Start launches the loop in a goroutine.
func (e *Engine) Start() {
	go func() {
		for {
			cfg := config.LoadConfig()
			interval := time.Duration(cfg.PollInterval) * time.Second
			if interval < 3*time.Second {
				interval = 3 * time.Second
			}
			e.Cycle()
			time.Sleep(interval)
		}
	}()
}

// LastCycle returns the latest cycle info (for /api/status).
func (e *Engine) LastCycle() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastCycle
}

func (e *Engine) setLastCycle(v map[string]any) {
	e.mu.Lock()
	e.lastCycle = v
	e.mu.Unlock()
}

func nowISO() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (e *Engine) dataClient(cfg config.Config) *exchange.Client {
	base := exchange.TestnetBase
	if cfg.PaperDataSource == "mainnet" {
		base = exchange.MainnetBase
	}
	return exchange.NewClient(base, cfg.BinanceAPIKey, cfg.BinanceAPISecret)
}

// Cycle is one engine iteration.
func (e *Engine) Cycle() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("engine cycle panic: %v", r)
		}
	}()

	cfg := config.LoadConfig()
	if cfg.ResetRequested {
		cfg.ResetRequested = false
		_ = config.SaveConfig(cfg)
		config.Reset(cfg)
		e.Log("INFO", "engine", "paper wallet reset")
	}
	if !cfg.BotRunning {
		e.setLastCycle(map[string]any{"running": false, "ts": nowISO()})
		return
	}

	st := config.LoadState(cfg)
	client := e.dataClient(cfg)
	mode := cfg.TradingMode
	if mode == "live" && (cfg.BinanceAPIKey == "" || cfg.BinanceAPISecret == "") {
		// KHÔNG fallback về paper — user yêu cầu chỉ nhận trade qua API
		// testnet thật. Thiếu keys → pause bot + log lỗi rõ ràng.
		e.Log("ERROR", "engine", "live mode được chọn nhưng thiếu BINANCE_API_KEY/BINANCE_API_SECRET — bot PAUSED (không fallback paper)")
		paused := cfg
		paused.BotRunning = false
		_ = config.SaveConfig(paused)
		e.setLastCycle(map[string]any{"running": false, "ts": nowISO(), "mode": mode, "error": "missing API credentials in live mode — bot paused"})
		return
	}

	symbols := parseSymbols(cfg.TradingSymbols)

	// 1) candles + prices + regimes
	candles := map[string][]exchange.Candle{}
	prices := map[string]float64{}
	for _, sym := range symbols {
		cd, err := client.Klines(sym, cfg.Timeframe, 200)
		if err != nil {
			e.Log("WARN", "data", fmt.Sprintf("klines %s failed: %v", sym, err))
			continue
		}
		candles[sym] = cd
		if len(cd) > 0 {
			prices[sym] = cd[len(cd)-1].Close
		}
		// regime classification mỗi cycle
		sn := regime.Classify(sym, cd)
		if prev, ok := e.Regimes()[sym]; ok {
			sn.Prev = prev.Regime
			sn.Changed = prev.Regime != sn.Regime
		}
		e.setRegime(sym, sn)
		if sn.Changed {
			e.Log("INFO", "regime", fmt.Sprintf("%s: %s → %s (ADX %.1f, ATR%% %.2f, conf %.2f)",
				sym, sn.Prev, sn.Regime, sn.Metrics.ADX, sn.Metrics.ATRRatio, sn.Confidence))
		}
	}

	// 2) manage open trades
	db := config.LoadDB()
	openTrades := []*config.Trade{}
	for i := range db.Trades {
		if db.Trades[i].Status == "open" {
			openTrades = append(openTrades, &db.Trades[i])
		}
	}
	for _, t := range openTrades {
		price, ok := prices[t.Symbol]
		if !ok {
			continue
		}
		e.manageTrade(cfg, mode, &st, t, price, candles[t.Symbol])
	}

	// 3) entries — adaptive: strategy theo regime từng symbol
	db = config.LoadDB()
	openSyms := map[string]bool{}
	openCount := 0
	for i := range db.Trades {
		if db.Trades[i].Status == "open" {
			openSyms[db.Trades[i].Symbol] = true
			openCount++
		}
	}
	adaptive := cfg.Strategy == "adaptive"
	for _, sym := range symbols {
		if openSyms[sym] {
			continue
		}
		// cooldown guard: symbol vừa stop_loss → chờ hết N phút
		if e.inCooldown(sym) {
			continue
		}
		cd := candles[sym]
		if len(cd) == 0 {
			continue
		}
		stratID := cfg.Strategy
		if adaptive {
			if sn, ok := e.Regimes()[sym]; ok {
				id, _ := regime.StrategyFor(sn.Regime)
				stratID = id
			}
		}
		// trend_down: long-only → không vào lệnh mới
		if adaptive {
			if sn, ok := e.Regimes()[sym]; ok && sn.Regime == regime.TrendDown {
				continue
			}
		}
		strat := strategy.Get(stratID)
		sig := strat.EntrySignal(cd)
		if sig == nil {
			continue
		}
		price := prices[sym]
		stake := e.stakeFor(cfg, st)
		if stake <= 0 {
			e.Log("WARN", "risk", "stake is 0, skipping entry")
			break
		}
		e.openTrade(cfg, mode, &st, sym, stake, price, stratID, sig.Reason)
		openSyms[sym] = true
		openCount++
		if openCount >= cfg.MaxOpenTrades {
			break
		}
	}

	// 4) circuit breaker: daily loss limit — mất ≥ X% trong ngày UTC → pause bot
	if cfg.DailyLossLimitPct > 0 {
		e.dailyLossCheck(cfg)
	}

	_ = config.SaveState(st)
	eq := equityOf(st, prices)
	eqMode := mode
	if mode == "live" {
		// live equity = tổng giá trị USDT của MỌI coin trên testnet account
		acc, err := e.liveClient(cfg).Account()
		if err != nil {
			e.Log("WARN", "account", fmt.Sprintf("account fetch failed: %v", err))
		} else if raw, ok := acc["balances"].([]any); ok {
			if all, err2 := client.AllPrices(); err2 == nil {
				_, total := exchange.ValueBalances(raw, all)
				eq = equityVal{Cash: total, PositionsValue: 0, Total: total}
				eqMode = "live"
			}
		}
	}
	_ = config.Mutate(func(c *config.Collection) {
		id := int64(len(c.Equity)) + 1
		c.Equity = append(c.Equity, config.EquityPoint{
			ID: id, TS: nowISO(), Equity: eq.Total, Cash: eq.Cash,
			PositionsValue: eq.PositionsValue, Mode: eqMode,
		})
	})

	rounded := map[string]float64{}
	for k, v := range prices {
		rounded[k] = round(v, 6)
	}
	e.setLastCycle(map[string]any{
		"running":   true,
		"ts":        nowISO(),
		"mode":      mode,
		"symbols":   symbols,
		"prices":    rounded,
		"strategy":  cfg.Strategy,
		"timeframe": cfg.Timeframe,
		"open_trades": openCount,
	})
}

func parseSymbols(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func round(v float64, places int) float64 {
	p := 1.0
	for i := 0; i < places; i++ {
		p *= 10
	}
	return float64(int64(v*p+0.5)) / p
}

func (e *Engine) stakeFor(cfg config.Config, st config.State) float64 {
	if cfg.StakeMode == "percent" {
		return round(st.Cash*cfg.StakePercent/100, 2)
	}
	return round(cfg.StakeAmount, 2)
}

type equityVal struct{ Cash, PositionsValue, Total float64 }

func equityOf(st config.State, prices map[string]float64) equityVal {
	posVal := 0.0
	for sym, pos := range st.Positions {
		p, ok := prices[sym]
		if !ok {
			p = pos.Cost
		}
		posVal += pos.Qty * p
	}
	return equityVal{Cash: st.Cash, PositionsValue: posVal, Total: st.Cash + posVal}
}

// ---------------------------------------------------------------------------
// trade ops
// ---------------------------------------------------------------------------

func (e *Engine) openTrade(cfg config.Config, mode string, st *config.State, symbol string, stake, price float64, stratName, reason string) {
	if mode == "paper" {
		if stake > st.Cash {
			e.Log("WARN", "risk", fmt.Sprintf("skip buy %s: insufficient cash", symbol))
			return
		}
		fee := stake * tradeFeeRate
		qty := (stake - fee) / price
		if pos, ok := st.Positions[symbol]; ok {
			totalCost := pos.Cost*pos.Qty + stake
			pos.Qty += qty
			pos.Cost = totalCost / pos.Qty
			st.Positions[symbol] = pos
		} else {
			st.Positions[symbol] = config.Position{Qty: qty, Cost: price}
		}
		st.Cash -= stake
		e.createTrade(cfg, mode, symbol, qty, price, stake, fee, stratName, reason)
		return
	}
	// live
	res, err := e.liveClient(cfg).MarketBuy(symbol, stake)
	if err != nil {
		e.Log("ERROR", "trade", fmt.Sprintf("BUY %s failed: %v", symbol, err))
		e.orderLog(symbol, "error", mode, nil, nil, "error", fmt.Sprintf("buy failed: %v", err))
		return
	}
	qty := toFloat(res["executedQty"])
	cum := toFloat(res["cummulativeQuoteQty"])
	if cum > 0 && qty > 0 {
		price = cum / qty
	}
	e.createTrade(cfg, mode, symbol, qty, price, stake, 0, stratName, reason)
}

func (e *Engine) liveClient(cfg config.Config) *exchange.Client {
	return exchange.NewClient(exchange.TestnetBase, cfg.BinanceAPIKey, cfg.BinanceAPISecret)
}

// orderLog appends an order-activity row without a trade id.
func (e *Engine) orderLog(symbol, action, mode string, qty, price *float64, status, detail string) {
	_ = config.Mutate(func(c *config.Collection) {
		c.Orders = append(c.Orders, config.OrderLog{
			ID: int64(len(c.Orders)) + 1, Symbol: symbol, Action: action, Mode: mode,
			Qty: qty, Price: price, Status: status, Detail: detail, CreatedAt: nowISO(),
		})
	})
}

// persistMeta writes trade meta + SL/TP back to the store.
func (e *Engine) persistMeta(t *config.Trade) {
	_ = config.Mutate(func(c *config.Collection) {
		for i := range c.Trades {
			if c.Trades[i].ID == t.ID {
				c.Trades[i].Meta = t.Meta
				c.Trades[i].StopLoss = t.StopLoss
				c.Trades[i].TakeProfit = t.TakeProfit
				break
			}
		}
	})
}

// createTrade writes a new open trade + buy order log row.
func (e *Engine) createTrade(cfg config.Config, mode, symbol string, qty, price, stake, fee float64, stratName, reason string) {
	sl := price * (1 - cfg.StopLossPct/100)
	tp := price * (1 + cfg.TakeProfitPct/100)
	t := config.Trade{
		Symbol: symbol, Side: "buy", Status: "open", Mode: mode,
		Qty: qty, EntryPrice: price, StopLoss: sl, TakeProfit: tp,
		Stake: stake, Fee: fee, Strategy: stratName, SignalReason: reason,
		OpenedAt: nowISO(),
		Meta: &config.TradeMeta{
			GridCount: 0, InitialStake: stake, LastAddPrice: price,
			TrailActivated: false, TrailHigh: price,
		},
	}
	_ = config.Mutate(func(c *config.Collection) {
		t.ID = int64(len(c.Trades)) + 1
		c.Trades = append(c.Trades, t)
		tid := t.ID
		qtyC, priceC := qty, price
		c.Orders = append(c.Orders, config.OrderLog{
			ID: int64(len(c.Orders)) + 1, TradeID: &tid, Symbol: symbol,
			Action: "buy", Mode: mode, Qty: &qtyC, Price: &priceC, Status: "ok",
			Detail: "entry: " + reason, CreatedAt: nowISO(),
		})
	})
	e.Log("INFO", "trade", fmt.Sprintf("BUY %s qty=%.6f @ %.2f (%s)", symbol, qty, price, reason))
}

// manageTrade: regime transition → trailing → exit checks → grid add.
func (e *Engine) manageTrade(cfg config.Config, mode string, st *config.State, t *config.Trade, price float64, candles []exchange.Candle) {
	if t.Meta == nil {
		t.Meta = &config.TradeMeta{InitialStake: t.Stake, TrailHigh: t.EntryPrice}
	}

	// 0) ADAPTIVE: regime transition rules cho lệnh đang mở
	if cfg.Strategy == "adaptive" {
		sn, ok := e.Regimes()[t.Symbol]
		if !ok {
			sn = regime.Classify(t.Symbol, candles)
		}
		if t.Meta.EntryRegime == "" {
			t.Meta.EntryRegime = string(sn.Regime)
			e.persistMeta(t)
		}
		entryRegime := regime.Regime(t.Meta.EntryRegime)
		nowRegime := sn.Regime
		if nowRegime != entryRegime || t.Meta.Converted {
			dec := regime.OnTransition(entryRegime, nowRegime, price > t.EntryPrice)
			if dec.Action == "exit" {
				e.Log("INFO", "regime", fmt.Sprintf("%s %s: %s", t.Symbol, dec.Action, dec.Reason))
				e.closeTrade(cfg, mode, st, t, price, "regime_shift:"+string(nowRegime))
				return
			}
			if dec.Action == "convert" && !t.Meta.Converted {
				// sideways lệnh long có lãi → trend_up: giữ, nới TP, trailing theo vol
				t.Meta.Converted = true
				t.Meta.EntryRegime = string(nowRegime)
				// nới TP theo trend: +1 R thêm (gap TP × 1.5)
				tpGap := (t.TakeProfit - t.EntryPrice) / t.EntryPrice
				if tpGap > 0 {
					t.TakeProfit = t.EntryPrice * (1 + tpGap*1.5)
				}
				// BẬT trailing theo biến động hiện tại
				trailPct := regime.TrailingParams(nowRegime, sn.Metrics.ATRRatio)
				t.Meta.VolTrailPct = trailPct
				e.Log("INFO", "regime", fmt.Sprintf("%s: CONVERT %s→%s — hold & trail %.2f%%, TP nới %.2f",
					t.Symbol, entryRegime, nowRegime, trailPct, t.TakeProfit))
				e.persistMeta(t)
			}
		}
	}

	// 1) trailing stop (ratchet up only) — adaptive: distance theo volatility
	if cfg.TrailingStop || t.Meta.VolTrailPct > 0 {
		trailPct := cfg.TrailingStopPct
		if t.Meta.VolTrailPct > 0 {
			trailPct = t.Meta.VolTrailPct
		}
		meta := t.Meta
		if price > meta.TrailHigh {
			meta.TrailHigh = price
		}
		gainPct := (price - t.EntryPrice) / t.EntryPrice
		trail := trailPct / 100
		if !meta.TrailActivated && gainPct >= trail*2 {
			meta.TrailActivated = true
		}
		if meta.TrailActivated {
			newSL := meta.TrailHigh * (1 - trail)
			if newSL > t.StopLoss {
				t.StopLoss = newSL
				e.Log("INFO", "trail", fmt.Sprintf("TRAIL %s: SL raised to %.4f (high %.4f, %.2f%%)",
					t.Symbol, newSL, meta.TrailHigh, trailPct))
			}
		}
		// persist trailing state
		e.persistMeta(t)
	}

	// 2) exits
	exitReason := ""
	if price <= t.StopLoss {
		if t.Meta.TrailActivated {
			exitReason = "trailing_stop"
		} else {
			exitReason = "stop_loss"
		}
	} else if price >= t.TakeProfit {
		exitReason = "take_profit"
	} else if len(candles) > 0 {
		strat := strategy.Get(t.Strategy)
		if s := strat.ExitSignal(candles, t); s != "" {
			exitReason = s
		}
	}
	if exitReason != "" {
		// cooldown: chỉ set khi thoát lỗ (stop_loss / trailing_stop / cut)
		if exitReason == "stop_loss" || exitReason == "trailing_stop" || strings.Contains(exitReason, "regime_shift") {
			if cfg.CooldownMinutes > 0 {
				e.setCooldown(t.Symbol, cfg.CooldownMinutes)
				e.Log("INFO", "risk", fmt.Sprintf("cooldown %s %d phút sau %s", t.Symbol, cfg.CooldownMinutes, exitReason))
			}
		}
		e.closeTrade(cfg, mode, st, t, price, exitReason)
		return
	}

	// 3) grid DCA add
	if len(candles) > 0 {
		strat := strategy.Get(t.Strategy)
		extra := strat.AdjustSignal(candles, t, cfg)
		if extra > 0 {
			e.gridAdd(cfg, mode, st, t, extra, price)
		}
	}
}

func (e *Engine) gridAdd(cfg config.Config, mode string, st *config.State, t *config.Trade, extra, price float64) {
	var addQty, addFee float64
	if mode == "paper" {
		if extra > st.Cash {
			e.Log("WARN", "grid", fmt.Sprintf("skip grid add %s: not enough cash", t.Symbol))
			return
		}
		addFee = extra * tradeFeeRate
		addQty = (extra - addFee) / price
		if pos, ok := st.Positions[t.Symbol]; ok {
			totalCost := pos.Cost*pos.Qty + extra
			pos.Qty += addQty
			pos.Cost = totalCost / pos.Qty
			st.Positions[t.Symbol] = pos
		}
		st.Cash -= extra
	} else {
		res, err := e.liveClient(cfg).MarketBuy(t.Symbol, extra)
		if err != nil {
			e.Log("ERROR", "grid", fmt.Sprintf("GRID ADD %s failed: %v", t.Symbol, err))
			return
		}
		addQty = toFloat(res["executedQty"])
		cum := toFloat(res["cummulativeQuoteQty"])
		if cum > 0 && addQty > 0 {
			price = cum / addQty
		}
	}

	totalCost := t.EntryPrice*t.Qty + extra
	newQty := t.Qty + addQty
	newAvg := totalCost / newQty
	slGap := (t.EntryPrice - t.StopLoss) / t.EntryPrice
	tpGap := (t.TakeProfit - t.EntryPrice) / t.EntryPrice

	_ = config.Mutate(func(c *config.Collection) {
		for i := range c.Trades {
			if c.Trades[i].ID == t.ID {
				tr := &c.Trades[i]
				tr.Qty = newQty
				tr.EntryPrice = newAvg
				tr.Stake += extra
				tr.Fee += addFee
				tr.StopLoss = newAvg * (1 - slGap)
				tr.TakeProfit = newAvg * (1 + tpGap)
				if tr.Meta != nil {
					tr.Meta.GridCount++
					tr.Meta.LastAddPrice = price
				}
				tid := tr.ID
				qtyC, priceC := addQty, price
				c.Orders = append(c.Orders, config.OrderLog{
					ID: int64(len(c.Orders)) + 1, TradeID: &tid, Symbol: t.Symbol,
					Action: "grid_add", Mode: mode, Qty: &qtyC, Price: &priceC, Status: "ok",
					Detail: fmt.Sprintf("DCA add level %d: +%.0f USDT, new avg %.4f", tr.Meta.GridCount, extra, newAvg),
					CreatedAt: nowISO(),
				})
				break
			}
		}
	})
	e.Log("INFO", "grid", fmt.Sprintf("GRID ADD %s +%.6f @ %.2f → avg %.4f", t.Symbol, addQty, price, newAvg))
}

func (e *Engine) closeTrade(cfg config.Config, mode string, st *config.State, t *config.Trade, price float64, reason string) {
	if mode == "paper" {
		if pos, ok := st.Positions[t.Symbol]; ok {
			if pos.Qty >= t.Qty-1e-12 {
				gross := t.Qty * price
				fee := gross * tradeFeeRate
				st.Cash += gross - fee
				pos.Qty -= t.Qty
				if pos.Qty <= 1e-12 {
					delete(st.Positions, t.Symbol)
				} else {
					st.Positions[t.Symbol] = pos
				}
			}
		}
	} else {
		res, err := e.liveClient(cfg).MarketSell(t.Symbol, t.Qty)
		if err != nil {
			e.Log("ERROR", "trade", fmt.Sprintf("SELL %s failed: %v", t.Symbol, err))
			return
		}
		qty := toFloat(res["executedQty"])
		cum := toFloat(res["cummulativeQuoteQty"])
		if cum > 0 && qty > 0 {
			price = cum / qty
		}
	}

	gross := t.Qty * price
	pnl := gross - t.Stake
	pnlPct := 0.0
	if t.Stake > 0 {
		pnlPct = pnl / t.Stake * 100
	}
	exitP := price
	closedAt := nowISO()
	_ = config.Mutate(func(c *config.Collection) {
		for i := range c.Trades {
			if c.Trades[i].ID == t.ID {
				c.Trades[i].Status = "closed"
				c.Trades[i].ExitPrice = &exitP
				c.Trades[i].PnL = pnl
				c.Trades[i].PnLPct = pnlPct
				c.Trades[i].ExitReason = reason
				c.Trades[i].ClosedAt = &closedAt
				tid := c.Trades[i].ID
				qtyC := c.Trades[i].Qty
				c.Orders = append(c.Orders, config.OrderLog{
					ID: int64(len(c.Orders)) + 1, TradeID: &tid, Symbol: t.Symbol,
					Action: "sell", Mode: mode, Qty: &qtyC, Price: &exitP, Status: "ok",
					Detail: fmt.Sprintf("exit (%s) pnl=%+.2f", reason, pnl), CreatedAt: nowISO(),
				})
				break
			}
		}
	})
	e.Log("INFO", "trade", fmt.Sprintf("SELL %s @ %.2f reason=%s pnl=%+.2f", t.Symbol, price, reason, pnl))
}

// toFloat converts Binance order fields, which arrive as JSON strings.
func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

// ---------------------------------------------------------------------------
// risk guards
// ---------------------------------------------------------------------------

// dailyLossCheck: circuit breaker — so equity hiện tại với equity điểm đầu tiên
// của ngày UTC. Nếu mất ≥ DailyLossLimitPct% → pause bot + log ERROR.
// Nếu limit đặt nhưng chưa có baseline ngày hôm nay (điểm equity đầu tiên của
// ngày), không pause — chỉ khi đã có dữ liệu mới so được.
func (e *Engine) dailyLossCheck(cfg config.Config) {
	db := config.LoadDB()
	today := time.Now().UTC().Format("2006-01-02")
	var dayStart *config.EquityPoint
	for i := range db.Equity {
		p := db.Equity[i]
		if strings.HasPrefix(p.TS, today) {
			dayStart = &db.Equity[i]
			break
		}
	}
	if dayStart == nil || dayStart.Equity <= 0 {
		return // chưa có baseline hôm nay
	}
	// equity mới nhất
	if len(db.Equity) == 0 {
		return
	}
	lastEq := db.Equity[len(db.Equity)-1].Equity
	lossPct := (dayStart.Equity - lastEq) / dayStart.Equity * 100
	if lossPct >= cfg.DailyLossLimitPct {
		paused := cfg
		paused.BotRunning = false
		if err := config.SaveConfig(paused); err == nil {
			e.Log("ERROR", "risk", fmt.Sprintf(
				"DAILY LOSS LIMIT: −%.2f%% hôm nay (limit %.2f%%) — bot PAUSED cho tới khi bạn bật lại",
				lossPct, cfg.DailyLossLimitPct))
		}
	}
}

// ---------------------------------------------------------------------------
// portfolio convert (500 coin → 1 coin)
// ---------------------------------------------------------------------------

// ConvertPreview liệt kê những coin nào sẽ được bán và ước tính USDT nhận được.
// mode="all": mọi coin non-USDT đủ điều kiện. mode="selected": chỉ assets đưa vào.
// minValue: bỏ qua coin dưới ngưỡng giá trị này (dust).
func (e *Engine) ConvertPreview(assets []string, mode string, minValue float64) ([]map[string]any, float64, error) {
	cfg := config.LoadConfig()
	if cfg.TradingMode != "live" || cfg.BinanceAPIKey == "" {
		return nil, 0, fmt.Errorf("convert chỉ hoạt động ở live mode (testnet) với API keys")
	}
	if minValue <= 0 {
		minValue = exchange.DustThreshold
	}
	client := e.liveClient(cfg)
	if err := client.LoadExchangeInfo(); err != nil {
		return nil, 0, fmt.Errorf("exchangeInfo: %w", err)
	}
	acc, err := client.Account()
	if err != nil {
		return nil, 0, err
	}
	raw, _ := acc["balances"].([]any)
	allPrices, _ := client.AllPrices()
	if allPrices == nil {
		allPrices = map[string]float64{}
	}
	rows, _ := exchange.ValueBalances(raw, allPrices)

	selected := map[string]bool{}
	for _, a := range assets {
		selected[strings.ToUpper(strings.TrimSpace(a))] = true
	}

	out := []map[string]any{}
	totalEst := 0.0
	for _, b := range rows {
		if b.Asset == "USDT" || b.Free <= 0 {
			continue
		}
		if mode == "selected" && !selected[b.Asset] {
			continue
		}
		price := b.Price
		if price <= 0 {
			if p, ok := allPrices[b.Asset+"USDT"]; ok {
				price = p
			}
		}
		_, qty, tradable := client.Sellable(b, price)
		estUSDT := exchange.SellQuoteValue(qty, price)
		row := map[string]any{
			"asset": b.Asset, "qty": qty, "price": price,
			"usdt_value": b.USDTValue, "est_net": round(estUSDT, 2),
			"tradable": tradable,
		}
		if !tradable {
			if b.USDTValue < minValue {
				row["skip_reason"] = "dust" // dưới ngưỡng giá trị
			} else if price <= 0 {
				row["skip_reason"] = "no_price"
			} else {
				row["skip_reason"] = "no_pair_or_filter"
			}
			out = append(out, row)
			continue
		}
		if estUSDT < minValue {
			row["tradable"] = false
			row["skip_reason"] = "dust"
			out = append(out, row)
			continue
		}
		totalEst += estUSDT
		out = append(out, row)
	}
	return out, round(totalEst, 2), nil
}

// ConvertExecute bán mọi coin non-USDT đủ điều kiện (all hoặc selected) về USDT.
// Trả về tổng USDT thực nhận + danh sách kết quả từng coin.
func (e *Engine) ConvertExecute(assets []string, mode string, minValue float64) ([]map[string]any, float64, error) {
	cfg := config.LoadConfig()
	if cfg.TradingMode != "live" || cfg.BinanceAPIKey == "" {
		return nil, 0, fmt.Errorf("convert chỉ hoạt động ở live mode (testnet) với API keys")
	}
	if minValue <= 0 {
		minValue = exchange.DustThreshold
	}
	client := e.liveClient(cfg)
	if err := client.LoadExchangeInfo(); err != nil {
		return nil, 0, fmt.Errorf("exchangeInfo: %w", err)
	}
	acc, err := client.Account()
	if err != nil {
		return nil, 0, err
	}
	raw, _ := acc["balances"].([]any)
	allPrices, _ := client.AllPrices()
	if allPrices == nil {
		allPrices = map[string]float64{}
	}
	rows, _ := exchange.ValueBalances(raw, allPrices)

	selected := map[string]bool{}
	for _, a := range assets {
		selected[strings.ToUpper(strings.TrimSpace(a))] = true
	}

	results := []map[string]any{}
	totalUSDT := 0.0
	converted := 0
	for _, b := range rows {
		if b.Asset == "USDT" || b.Free <= 0 {
			continue
		}
		if mode == "selected" && !selected[b.Asset] {
			continue
		}
		price := b.Price
		if price <= 0 {
			if p, ok := allPrices[b.Asset+"USDT"]; ok {
				price = p
			}
		}
		sym, qty, tradable := client.Sellable(b, price)
		if !tradable {
			continue
		}
		estNet := exchange.SellQuoteValue(qty, price)
		if estNet < minValue {
			continue // dust
		}
		res, err := client.MarketSell(sym, qty)
		row := map[string]any{"asset": b.Asset, "symbol": sym, "qty": qty}
		if err != nil {
			row["ok"] = false
			row["error"] = err.Error()
			e.Log("ERROR", "convert", fmt.Sprintf("sell %s failed: %v", sym, err))
			results = append(results, row)
			continue
		}
		execQty := toFloat(res["executedQty"])
		cum := toFloat(res["cummulativeQuoteQty"])
		got := cum * (1 - tradeFeeRate)
		row["ok"] = true
		row["executed_qty"] = execQty
		row["gross_usdt"] = round(cum, 2)
		row["net_usdt"] = round(got, 2)
		totalUSDT += got
		converted++
		e.Log("INFO", "convert", fmt.Sprintf("CONVERT %s → %.2f USDT (qty %.8f)", sym, got, execQty))
		results = append(results, row)
	}
	e.Log("INFO", "convert", fmt.Sprintf("convert done: %d coins → %.2f USDT", converted, totalUSDT))
	return results, round(totalUSDT, 2), nil
}

// ---------------------------------------------------------------------------
// partial close
// ---------------------------------------------------------------------------

// PartialSell đóng một phần lệnh đang mở (pct 1–100). Còn lại giữ nguyên SL/TP.
func (e *Engine) PartialSell(tradeID int64, pct float64) (float64, float64, error) {
	if pct <= 0 || pct > 100 {
		return 0, 0, fmt.Errorf("pct phải trong 1–100")
	}
	cfg := config.LoadConfig()
	st := config.LoadState(cfg)
	mode := cfg.TradingMode
	if mode == "live" && (cfg.BinanceAPIKey == "" || cfg.BinanceAPISecret == "") {
		return 0, 0, fmt.Errorf("live mode thiếu API keys — lệnh bị từ chối")
	}
	db := config.LoadDB()
	var t *config.Trade
	for i := range db.Trades {
		if db.Trades[i].ID == tradeID && db.Trades[i].Status == "open" {
			t = &db.Trades[i]
			break
		}
	}
	if t == nil {
		return 0, 0, fmt.Errorf("trade not found or already closed")
	}
	client := e.dataClient(cfg)
	price, err := client.TickerPrice(t.Symbol)
	if err != nil {
		return 0, 0, err
	}

	sellQty := t.Qty * pct / 100
	if sellQty <= 0 {
		return 0, 0, fmt.Errorf("sell qty = 0")
	}

	var gross, fee float64
	if mode == "paper" {
		if pos, ok := st.Positions[t.Symbol]; ok {
			gross = sellQty * price
			fee = gross * tradeFeeRate
			st.Cash += gross - fee
			pos.Qty -= sellQty
			if pos.Qty <= 1e-12 {
				delete(st.Positions, t.Symbol)
			} else {
				st.Positions[t.Symbol] = pos
			}
		}
	} else {
		// làm tròn theo LOT_SIZE của sàn
		if err := client.LoadExchangeInfo(); err == nil {
			if lf, ok := client.LotSize(t.Symbol); ok && lf.StepSize > 0 {
				sellQty = lf.RoundQty(sellQty)
			}
		}
		if sellQty <= 0 {
			return 0, 0, fmt.Errorf("sau LOT_SIZE rounding, qty = 0")
		}
		res, err := e.liveClient(cfg).MarketSell(t.Symbol, sellQty)
		if err != nil {
			return 0, 0, err
		}
		execQty := toFloat(res["executedQty"])
		cum := toFloat(res["cummulativeQuoteQty"])
		if cum > 0 && execQty > 0 {
			gross = cum
			fee = cum * tradeFeeRate
		} else {
			gross = execQty * price
			fee = gross * tradeFeeRate
		}
	}

	// cập nhật trade: qty còn lại, stake proportional, order log
	remainQty := t.Qty - sellQty
	remainStake := t.Stake * (1 - pct/100)
	net := gross - fee
	_ = config.Mutate(func(c *config.Collection) {
		for i := range c.Trades {
			if c.Trades[i].ID == t.ID {
				tr := &c.Trades[i]
				tr.Qty = remainQty
				tr.Stake = remainStake
				tid := tr.ID
				qtyC, priceC := sellQty, price
				c.Orders = append(c.Orders, config.OrderLog{
					ID: int64(len(c.Orders)) + 1, TradeID: &tid, Symbol: t.Symbol,
					Action: "partial_sell", Mode: mode, Qty: &qtyC, Price: &priceC, Status: "ok",
					Detail: fmt.Sprintf("partial close %.0f%%: sold %.6f @ %.2f, net %.2f", pct, sellQty, price, net),
					CreatedAt: nowISO(),
				})
				break
			}
		}
	})
	_ = config.SaveState(st)
	e.Log("INFO", "trade", fmt.Sprintf("PARTIAL SELL %s %.0f%%: %.6f @ %.2f (net %.2f, remaining %.6f)",
		t.Symbol, pct, sellQty, price, gross-fee, remainQty))
	return sellQty, gross - fee, nil
}



// Log appends an engine log entry.
func (e *Engine) Log(level, module, message string) {
	_ = config.Mutate(func(c *config.Collection) {
		c.Logs = append(c.Logs, config.LogEntry{
			ID: int64(len(c.Logs)) + 1, Level: level, Module: module,
			Message: message, CreatedAt: nowISO(),
		})
		// cap logs at 1000 entries
		if len(c.Logs) > 1000 {
			c.Logs = c.Logs[len(c.Logs)-1000:]
		}
	})
}

// ---------------------------------------------------------------------------
// manual ops (API)
// ---------------------------------------------------------------------------

// ManualBuy opens a market buy from the dashboard.
func (e *Engine) ManualBuy(symbol string, stake float64) (float64, error) {
	cfg := config.LoadConfig()
	st := config.LoadState(cfg)
	mode := cfg.TradingMode
	if mode == "live" && (cfg.BinanceAPIKey == "" || cfg.BinanceAPISecret == "") {
		return 0, fmt.Errorf("live mode thiếu API keys — lệnh bị từ chối (không fallback paper)")
	}
	client := e.dataClient(cfg)
	price, err := client.TickerPrice(symbol)
	if err != nil {
		return 0, err
	}
	e.openTrade(cfg, mode, &st, strings.ToUpper(symbol), round(stake, 2), price, "manual", "manual order")
	_ = config.SaveState(st)
	return price, nil
}

// ManualSell closes a trade from the dashboard.
func (e *Engine) ManualSell(tradeID int64) error {
	cfg := config.LoadConfig()
	st := config.LoadState(cfg)
	mode := cfg.TradingMode
	if mode == "live" && (cfg.BinanceAPIKey == "" || cfg.BinanceAPISecret == "") {
		return fmt.Errorf("live mode thiếu API keys — lệnh bị từ chối (không fallback paper)")
	}
	var t *config.Trade
	db := config.LoadDB()
	for i := range db.Trades {
		if db.Trades[i].ID == tradeID && db.Trades[i].Status == "open" {
			t = &db.Trades[i]
			break
		}
	}
	if t == nil {
		return fmt.Errorf("trade not found or already closed")
	}
	client := e.dataClient(cfg)
	price, err := client.TickerPrice(t.Symbol)
	if err != nil {
		return err
	}
	e.closeTrade(cfg, mode, &st, t, price, "manual")
	_ = config.SaveState(st)
	return nil
}
