// Package api — REST endpoints for the dashboard (same contract as the Python version).
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"binance-trade-bot/internal/config"
	"binance-trade-bot/internal/engine"
	"binance-trade-bot/internal/exchange"
	"binance-trade-bot/internal/regime"
	"binance-trade-bot/internal/strategy"
)

// Mux builds the full router.
func Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/candles", handleCandles)
	mux.HandleFunc("/api/symbols", handleSymbols)
	mux.HandleFunc("/api/ticker", handleTicker)
	mux.HandleFunc("/api/trades", handleTrades)
	mux.HandleFunc("/api/activity", handleActivity)
	mux.HandleFunc("/api/logs", handleLogs)
	mux.HandleFunc("/api/equity", handleEquity)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/buy", handleBuy)
	mux.HandleFunc("/api/sell/", handleSell)
	mux.HandleFunc("/api/reset", handleReset)
	mux.HandleFunc("/api/account", handleAccount)
	mux.HandleFunc("/api/regimes", handleRegimes)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "service": "binance-trade-bot (go)"})
	})
	return mux
}

// CORS middleware.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": msg})
}

func dataClient() *exchange.Client {
	cfg := config.LoadConfig()
	base := exchange.TestnetBase
	if cfg.PaperDataSource == "mainnet" {
		base = exchange.MainnetBase
	}
	return exchange.NewClient(base, cfg.BinanceAPIKey, cfg.BinanceAPISecret)
}

// ---------------------------------------------------------------------------
// status
// ---------------------------------------------------------------------------

func handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg := config.LoadConfig()
	st := config.LoadState(cfg)
	last := engine.Get().LastCycle()
	prices := map[string]float64{}
	if p, ok := last["prices"].(map[string]float64); ok {
		prices = p
	}

	db := config.LoadDB()
	openTrades := []config.Trade{}
	var closed []config.Trade
	for _, t := range db.Trades {
		if t.Status == "open" {
			openTrades = append(openTrades, t)
		} else {
			closed = append(closed, t)
		}
	}

	posVal := 0.0
	for sym, pos := range st.Positions {
		p, ok := prices[sym]
		if !ok {
			p = pos.Cost
		}
		posVal += pos.Qty * p
	}
	equity := st.Cash + posVal
	startBal := st.InitBalance
	if startBal == 0 {
		startBal = cfg.StartBalance
	}

	wins, losses := 0, 0
	totalPnL, best, worst := 0.0, 0.0, 0.0
	var sumWin, sumLoss float64
	for _, t := range closed {
		if t.PnL > 0 {
			wins++
			sumWin += t.PnL
		} else {
			losses++
			sumLoss += t.PnL
		}
		totalPnL += t.PnL
		if t.PnL > best {
			best = t.PnL
		}
		if t.PnL < worst {
			worst = t.PnL
		}
	}
	winRate := 0.0
	if len(closed) > 0 {
		winRate = float64(wins) / float64(len(closed)) * 100
	}
	avgWin, avgLoss := 0.0, 0.0
	if wins > 0 {
		avgWin = sumWin / float64(wins)
	}
	if losses > 0 {
		avgLoss = sumLoss / float64(losses)
	}

	openOut := make([]map[string]any, 0, len(openTrades))
	for _, t := range openTrades {
		openOut = append(openOut, tradeJSON(t))
	}

	// live mode: số dư thật trên testnet (nhiều coin) thay vì ví paper
	liveBalances := []map[string]any{}
	liveEquity, liveCash := 0.0, 0.0
	if cfg.TradingMode == "live" && cfg.BinanceAPIKey != "" {
		client := exchange.NewClient(exchange.TestnetBase, cfg.BinanceAPIKey, cfg.BinanceAPISecret)
		if acc, err := client.Account(); err == nil {
			if raw, ok := acc["balances"].([]any); ok {
				allPrices := map[string]float64{}
				if all, err2 := client.AllPrices(); err2 == nil {
					allPrices = all
				}
				rows, total := exchange.ValueBalances(raw, allPrices)
				liveEquity = total
				for _, row := range rows {
					if row.Asset == "USDT" {
						liveCash = row.Free + row.Locked
					}
					liveBalances = append(liveBalances, map[string]any{
						"asset": row.Asset, "free": row.Free, "locked": row.Locked,
						"price": row.Price, "usdt_value": round2(row.USDTValue),
					})
				}
				// baseline: equity testnet lần đầu vào live mode → profit đếm từ đây
				if liveEquity > 0 && (st.LiveBaseline == 0 || st.LiveBaseline < liveEquity*0.5 || st.LiveBaseline > liveEquity*2) {
					st.LiveBaseline = liveEquity
					_ = config.SaveState(st)
				}
			}
		}
	}
	if liveEquity > 0 {
		equity = liveEquity
		posVal = liveEquity - liveCash
		if st.LiveBaseline > 0 {
			startBal = st.LiveBaseline
		}
	}

	cashOut := st.Cash
	if liveCash > 0 {
		cashOut = liveCash
	}

	writeJSON(w, map[string]any{
		"running":        cfg.BotRunning,
		"mode":           cfg.TradingMode,
		"last_cycle":     last,
		"equity":         round2(equity),
		"cash":           round2(cashOut),
		"positions_value": round2(posVal),
		"start_balance":  startBal,
		"profit":        round2(equity - startBal),
		"profit_pct":    round2((equity - startBal) / startBal * 100),
		"balances":      liveBalances,
		"open_trades":    openOut,
		"stats": map[string]any{
			"total_trades":  len(db.Trades),
			"closed_trades": len(closed),
			"wins":          wins,
			"losses":        losses,
			"win_rate":      round2(winRate),
			"total_pnl":     round2(totalPnL),
			"avg_win":       round2(avgWin),
			"avg_loss":      round2(avgLoss),
			"max_drawdown":  maxDrawdown(db.Equity),
			"best_trade":    round2(best),
			"worst_trade":   round2(worst),
		},
	})
}

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

func tradeJSON(t config.Trade) map[string]any {
	m := map[string]any{
		"id": t.ID, "symbol": t.Symbol, "side": t.Side, "status": t.Status, "mode": t.Mode,
		"qty": t.Qty, "entry_price": t.EntryPrice, "stop_loss": t.StopLoss, "take_profit": t.TakeProfit,
		"stake": t.Stake, "pnl": t.PnL, "pnl_pct": t.PnLPct, "fee": t.Fee,
		"strategy": t.Strategy, "signal_reason": t.SignalReason, "exit_reason": t.ExitReason,
		"opened_at": t.OpenedAt,
	}
	if t.ExitPrice != nil {
		m["exit_price"] = *t.ExitPrice
	} else {
		m["exit_price"] = nil
	}
	if t.ClosedAt != nil {
		m["closed_at"] = *t.ClosedAt
	} else {
		m["closed_at"] = nil
	}
	if t.Meta != nil {
		m["meta"] = t.Meta
	} else {
		m["meta"] = map[string]any{}
	}
	return m
}

func maxDrawdown(pts []config.EquityPoint) float64 {
	if len(pts) < 2 {
		return 0
	}
	peak, mdd := pts[0].Equity, 0.0
	for _, p := range pts {
		if p.Equity > peak {
			peak = p.Equity
		}
		if peak > 0 {
			dd := (peak - p.Equity) / peak * 100
			if dd > mdd {
				mdd = dd
			}
		}
	}
	return round2(mdd)
}

// ---------------------------------------------------------------------------
// market data
// ---------------------------------------------------------------------------

func handleCandles(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(orDefault(r.URL.Query().Get("symbol"), "BTCUSDT"))
	interval := orDefault(r.URL.Query().Get("interval"), "5m")
	limit, _ := strconv.Atoi(orDefault(r.URL.Query().Get("limit"), "200"))
	if limit > 500 {
		limit = 500
	}
	cd, err := dataClient().Klines(symbol, interval, limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"symbol": symbol, "interval": interval, "candles": cd})
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func handleSymbols(w http.ResponseWriter, r *http.Request) {
	syms, err := dataClient().ExchangeSymbols("USDT")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"symbols": syms})
}

func handleTicker(w http.ResponseWriter, r *http.Request) {
	in := orDefault(r.URL.Query().Get("symbols"), "BTCUSDT,ETHUSDT")
	out := map[string]float64{}
	client := dataClient()
	for _, s := range strings.Split(in, ",") {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if p, err := client.TickerPrice(s); err == nil {
			out[s] = p
		}
	}
	writeJSON(w, map[string]any{"prices": out})
}

// ---------------------------------------------------------------------------
// data endpoints
// ---------------------------------------------------------------------------

func handleTrades(w http.ResponseWriter, r *http.Request) {
	limit := limitOf(r, 100, 500)
	statusFilter := r.URL.Query().Get("status")
	db := config.LoadDB()
	out := []map[string]any{}
	for i := len(db.Trades) - 1; i >= 0 && len(out) < limit; i-- {
		t := db.Trades[i]
		if statusFilter != "" && t.Status != statusFilter {
			continue
		}
		out = append(out, tradeJSON(t))
	}
	writeJSON(w, map[string]any{"trades": out})
}

func limitOf(r *http.Request, def, max int) int {
	v, _ := strconv.Atoi(orDefault(r.URL.Query().Get("limit"), strconv.Itoa(def)))
	if v <= 0 || v > max {
		return def
	}
	return v
}

func handleActivity(w http.ResponseWriter, r *http.Request) {
	limit := limitOf(r, 100, 500)
	db := config.LoadDB()
	out := []config.OrderLog{}
	for i := len(db.Orders) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, db.Orders[i])
	}
	writeJSON(w, map[string]any{"activity": out})
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	limit := limitOf(r, 100, 500)
	level := strings.ToUpper(r.URL.Query().Get("level"))
	db := config.LoadDB()
	out := []config.LogEntry{}
	for i := len(db.Logs) - 1; i >= 0 && len(out) < limit; i-- {
		l := db.Logs[i]
		if level != "" && l.Level != level {
			continue
		}
		out = append(out, l)
	}
	writeJSON(w, map[string]any{"logs": out})
}

func handleEquity(w http.ResponseWriter, r *http.Request) {
	limit := limitOf(r, 300, 2000)
	db := config.LoadDB()
	n := len(db.Equity)
	if n > limit {
		db.Equity = db.Equity[n-limit:]
	}
	writeJSON(w, map[string]any{"points": db.Equity})
}

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := config.LoadConfig()
		writeJSON(w, map[string]any{
			"trading_mode":         cfg.TradingMode,
			"paper_data_source":    cfg.PaperDataSource,
			"start_balance":        cfg.StartBalance,
			"trading_symbols":      cfg.TradingSymbols,
			"timeframe":            cfg.Timeframe,
			"strategy":             cfg.Strategy,
			"stake_mode":           cfg.StakeMode,
			"stake_amount":         cfg.StakeAmount,
			"stake_percent":        cfg.StakePercent,
			"stop_loss_pct":        cfg.StopLossPct,
			"take_profit_pct":      cfg.TakeProfitPct,
			"max_open_trades":     cfg.MaxOpenTrades,
			"poll_interval":        cfg.PollInterval,
			"trailing_stop":        cfg.TrailingStop,
			"trailing_stop_pct":   cfg.TrailingStopPct,
			"grid_levels":         cfg.GridLevels,
			"bot_running":          cfg.BotRunning,
			"strategies_available": strategy.List(),
			"has_credentials":      cfg.BinanceAPIKey != "",
			"timeframes":           []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "1d"},
		})
	case http.MethodPost:
		var patch map[string]any
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeErr(w, 400, "invalid json: "+err.Error())
			return
		}
		cfg := config.LoadConfig()
		updated := map[string]any{}
		apply := func(key string, fn func()) {
			if v, ok := patch[key]; ok {
				fn()
				updated[key] = v
			}
		}
		apply("trading_mode", func() { cfg.TradingMode = str(patch["trading_mode"], cfg.TradingMode) })
		apply("paper_data_source", func() { cfg.PaperDataSource = str(patch["paper_data_source"], cfg.PaperDataSource) })
		apply("start_balance", func() { cfg.StartBalance = num(patch["start_balance"], cfg.StartBalance) })
		apply("trading_symbols", func() { cfg.TradingSymbols = strings.ToUpper(str(patch["trading_symbols"], cfg.TradingSymbols)) })
		apply("timeframe", func() { cfg.Timeframe = str(patch["timeframe"], cfg.Timeframe) })
		apply("strategy", func() { cfg.Strategy = str(patch["strategy"], cfg.Strategy) })
		apply("stake_mode", func() { cfg.StakeMode = str(patch["stake_mode"], cfg.StakeMode) })
		apply("stake_amount", func() { cfg.StakeAmount = num(patch["stake_amount"], cfg.StakeAmount) })
		apply("stake_percent", func() { cfg.StakePercent = num(patch["stake_percent"], cfg.StakePercent) })
		apply("stop_loss_pct", func() { cfg.StopLossPct = num(patch["stop_loss_pct"], cfg.StopLossPct) })
		apply("take_profit_pct", func() { cfg.TakeProfitPct = num(patch["take_profit_pct"], cfg.TakeProfitPct) })
		apply("max_open_trades", func() { cfg.MaxOpenTrades = int(num(patch["max_open_trades"], float64(cfg.MaxOpenTrades))) })
		apply("poll_interval", func() { cfg.PollInterval = int(num(patch["poll_interval"], float64(cfg.PollInterval))) })
		apply("trailing_stop", func() { cfg.TrailingStop = fmt.Sprint(patch["trailing_stop"]) == "true" })
		apply("trailing_stop_pct", func() { cfg.TrailingStopPct = num(patch["trailing_stop_pct"], cfg.TrailingStopPct) })
		apply("grid_levels", func() { cfg.GridLevels = int(num(patch["grid_levels"], float64(cfg.GridLevels))) })
		apply("bot_running", func() { cfg.BotRunning = fmt.Sprint(patch["bot_running"]) == "true" })
		if err := config.SaveConfig(cfg); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "updated": updated})
	default:
		writeErr(w, 405, "method not allowed")
	}
}

func str(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func num(v any, def float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return f
		}
	}
	return def
}

// ---------------------------------------------------------------------------
// manual trading
// ---------------------------------------------------------------------------

func handleBuy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var body struct {
		Symbol string  `json:"symbol"`
		Stake  float64 `json:"stake"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid json")
		return
	}
	if body.Symbol == "" || body.Stake <= 0 {
		writeErr(w, 400, "symbol and stake required")
		return
	}
	price, err := engine.Get().ManualBuy(body.Symbol, body.Stake)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "price": price})
}

func handleSell(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/sell/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid trade id")
		return
	}
	if err := engine.Get().ManualSell(id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	cfg := config.LoadConfig()
	config.Reset(cfg)
	engine.Get().Log("INFO", "engine", "paper wallet reset")
	writeJSON(w, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// account (live mode)
// ---------------------------------------------------------------------------

// regimes (adaptive engine)
// ---------------------------------------------------------------------------

func handleRegimes(w http.ResponseWriter, r *http.Request) {
	snaps := engine.Get().Regimes()
	out := make([]map[string]any, 0, len(snaps))
	for _, sn := range snaps {
		id, label := regime.StrategyFor(sn.Regime)
		out = append(out, map[string]any{
			"symbol": sn.Symbol, "regime": sn.Regime, "prev": sn.Prev,
			"changed": sn.Changed, "confidence": sn.Confidence,
			"metrics": sn.Metrics,
			"strategy": id, "strategy_label": label,
		})
	}
	sort.Slice(out, func(i, j int) bool { return str(out[i]["symbol"], "") < str(out[j]["symbol"], "") })
	writeJSON(w, map[string]any{"regimes": out})
}

func handleAccount(w http.ResponseWriter, r *http.Request) {
	cfg := config.LoadConfig()
	if cfg.TradingMode != "live" || cfg.BinanceAPIKey == "" {
		writeJSON(w, map[string]any{"live": false, "balances": []any{}})
		return
	}
	client := exchange.NewClient(exchange.TestnetBase, cfg.BinanceAPIKey, cfg.BinanceAPISecret)
	acc, err := client.Account()
	if err != nil {
		writeJSON(w, map[string]any{"live": false, "error": err.Error(), "balances": []any{}})
		return
	}
	balances := []map[string]any{}
	total := 0.0
	if bs, ok := acc["balances"].([]any); ok {
		allPrices := map[string]float64{}
		if all, err2 := client.AllPrices(); err2 == nil {
			allPrices = all
		}
		rows, tot := exchange.ValueBalances(bs, allPrices)
		total = tot
		for _, row := range rows {
			balances = append(balances, map[string]any{
				"asset": row.Asset, "free": row.Free, "locked": row.Locked,
				"price": row.Price, "usdt_value": round2(row.USDTValue),
			})
		}
	}
	writeJSON(w, map[string]any{"live": true, "total_usdt": round2(total), "balances": balances})
}
