// Package engine — the trading loop (paper/live, trailing, grid DCA).
package engine

import (
	"fmt"
	"log"
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
}

var defaultEngine *Engine

// Get returns the singleton engine.
func Get() *Engine {
	if defaultEngine == nil {
		defaultEngine = &Engine{
			lastCycle: map[string]any{},
			regimes:   map[string]regime.Snapshot{},
		}
	}
	return defaultEngine
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

	// 4) equity snapshot
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

func toFloat(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// ---------------------------------------------------------------------------
// logs
// ---------------------------------------------------------------------------

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
