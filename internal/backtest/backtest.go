// Package backtest — chạy strategy trên dữ liệu lịch sử, mô phỏng lại
// logic của engine thật (entry signal, SL/TP, trailing ratchet, grid DCA,
// cooldown) để đo hiệu năng trước khi chạy thật.
package backtest

import (
	"fmt"
	"math"
	"sort"

	"binance-trade-bot/internal/config"
	"binance-trade-bot/internal/exchange"
	"binance-trade-bot/internal/strategy"
	"binance-trade-bot/internal/ta"
)

// Params cấu hình 1 lần backtest.
type Params struct {
	Symbol       string
	Candles      []exchange.Candle // đủ dài (khuyến nghị ≥ 500)
	Strategy     string
	StartBalance float64
	Stake        float64
	StopLossPct  float64 // %
	TakeProfitPct float64 // %
	TrailingStop bool
	TrailingPct  float64 // %
	GridLevels   int
	CooldownBars int // số nến chờ sau stop_loss (0 = off)
	FeeRate      float64 // mỗi bên, mặc định 0.001

	// multi-symbol (tùy chọn): correlation filter
	OtherCandles map[string][]exchange.Candle
	MaxCorr      float64 // ngưỡng |ρ| chặn entry (0 = off), so với các vị thế đang mở
}

// Result kết quả 1 lần backtest.
type Result struct {
	Symbol       string  `json:"symbol"`
	Strategy     string  `json:"strategy"`
	Bars         int     `json:"bars"`
	Trades       int     `json:"trades"`
	Wins         int     `json:"wins"`
	Losses       int     `json:"losses"`
	WinRate      float64 `json:"win_rate"`   // %
	TotalPnL      float64 `json:"total_pnl"`
	TotalPnLPct  float64 `json:"total_pnl_pct"` // % so start balance
	MaxDrawdown  float64 `json:"max_drawdown"`  // %
	ProfitFactor float64 `json:"profit_factor"`  // grossWin / grossLoss (loss=0 → +Inf hiển thị 999)
	AvgWin       float64 `json:"avg_win"`
	AvgLoss      float64 `json:"avg_loss"`
	Expectancy   float64 `json:"expectancy"` // USDT trung bình mỗi lệnh
	Sharpe       float64 `json:"sharpe"`    // per-trade, annualized giả định không áp
	FinalEquity  float64 `json:"final_equity"`
}

// Run thực hiện backtest 1 symbol. Quan trọng: chỉ quyết định trên nến i
// khi biết dữ liệu tới nến i (không look-ahead) — entry/exit tính ở close
// của nến hiện tại, đúng cách engine thật hành động theo close.
func Run(p Params) Result {
	fee := p.FeeRate
	if fee <= 0 {
		fee = 0.001
	}
	strat := strategy.Get(p.Strategy)
	if p.Strategy == "adaptive" {
		// backtest đơn giản: adaptive cần regime — map tương đối bằng chính
		// candles: dùng ema_cross làm proxy cho adaptive
		strat = strategy.Get("ema_cross")
	}

	cash := p.StartBalance
	peak := p.StartBalance
	mdd := 0.0

	type openPos struct {
		entry, qty, stake, sl, tp   float64
		trailHigh, trailPct         float64
		trailOn                     bool
		gridCount                   int
		initialStake                float64
	}
	var pos *openPos

	closedPnL := []float64{}
	grossWin, grossLoss := 0.0, 0.0
	wins, losses := 0, 0
	cooldownUntil := -1

	cd := p.Candles
	n := len(cd)

	for i := 0; i < n; i++ {
		price := cd[i].Close
		upTo := cd[:i+1] // nến tới i (inclusive) — không look-ahead

		// --- quản lý vị thế đang mở ---
		if pos != nil {
			// trailing ratchet
			if p.TrailingStop && pos.trailPct > 0 {
				if price > pos.trailHigh {
					pos.trailHigh = price
				}
				gain := (price - pos.entry) / pos.entry
				tr := pos.trailPct / 100
				if !pos.trailOn && gain >= tr*2 {
					pos.trailOn = true
				}
				if pos.trailOn {
					newSL := pos.trailHigh * (1 - tr)
					if newSL > pos.sl {
						pos.sl = newSL
					}
				}
			}
			// exit checks (theo close nến, giống engine)
			reason := ""
			if price <= pos.sl {
				if pos.trailOn {
					reason = "trailing_stop"
				} else {
					reason = "stop_loss"
				}
			} else if price >= pos.tp {
				reason = "take_profit"
			} else if s := strat.ExitSignal(upTo, &config.Trade{EntryPrice: pos.entry, Qty: pos.qty, Stake: pos.stake}); s != "" {
				reason = s
			}
			if reason != "" {
				gross := pos.qty * price
				net := gross * (1 - fee)
				pnl := net - pos.stake
				cash += net
				closedPnL = append(closedPnL, pnl)
				if pnl > 0 {
					wins++
					grossWin += pnl
				} else {
					losses++
					grossLoss += -pnl
				}
				if reason == "stop_loss" || reason == "trailing_stop" {
					cooldownUntil = i + p.CooldownBars
				}
				pos = nil
			} else if p.GridLevels > 0 && p.Strategy == "adaptive_grid" {
				// grid DCA add (chỉ adaptive_grid, như engine)
				tr := &config.Trade{
					EntryPrice: pos.entry, Qty: pos.qty, Stake: pos.stake,
					Meta: &config.TradeMeta{GridCount: pos.gridCount, InitialStake: pos.initialStake},
				}
				if extra := strat.AdjustSignal(upTo, tr, config.Config{GridLevels: p.GridLevels}); extra > 0 && extra <= cash {
					qty := (extra * (1 - fee)) / price
					totalCost := pos.entry*pos.qty + extra
					pos.qty += qty
					pos.entry = totalCost / pos.qty
					pos.stake += extra
					pos.gridCount++
					cash -= extra
				}
			}
		}

		// --- entry mới ---
		// (strategy tự yêu cầu đủ nến — không cần gate cứng)
		if pos == nil && i > cooldownUntil && cash >= p.Stake && p.Stake > 0 {
			// correlation filter (đơn giản: nếu có vị thế song song khác symbol)
			// — trong backtest 1 symbol, lọc theo chính nó không cần thiết;
			// giữ hook cho multi-symbol extension.
			sig := strat.EntrySignal(upTo)
			if sig != nil {
				feeAmt := p.Stake * fee
				qty := (p.Stake - feeAmt) / price
				pos = &openPos{
					entry: price, qty: qty, stake: p.Stake,
					sl: price * (1 - p.StopLossPct/100),
					tp: price * (1 + p.TakeProfitPct/100),
					trailHigh: price, trailPct: p.TrailingPct,
					initialStake: p.Stake,
				}
				cash -= p.Stake
			}
		}

		// --- equity + drawdown ---
		eq := cash
		if pos != nil {
			eq += pos.qty * price
		}
		if eq > peak {
			peak = eq
		}
		if peak > 0 {
			dd := (peak - eq) / peak * 100
			if dd > mdd {
				mdd = dd
			}
		}
	}

	res := Result{
		Symbol:  p.Symbol,
		Strategy: p.Strategy,
		Bars:    n,
		Trades:  wins + losses,
		Wins:    wins,
		Losses:  losses,
		FinalEquity: cash,
	}
	if pos != nil {
		// vị thế còn mở cuối kỳ — tính theo giá close cuối
		last := cd[n-1].Close
		res.FinalEquity = cash + pos.qty*last
	}
	if res.Trades > 0 {
		res.WinRate = float64(wins) / float64(res.Trades) * 100
	}
	res.TotalPnL = res.FinalEquity - p.StartBalance
	res.TotalPnLPct = res.TotalPnL / p.StartBalance * 100
	res.MaxDrawdown = mdd
	if wins > 0 {
		res.AvgWin = grossWin / float64(wins)
	}
	if losses > 0 {
		res.AvgLoss = grossLoss / float64(losses)
	}
	res.Expectancy = 0
	if res.Trades > 0 {
		res.Expectancy = res.TotalPnL / float64(res.Trades)
	}
	if grossLoss > 0 {
		res.ProfitFactor = grossWin / grossLoss
	} else if grossWin > 0 {
		res.ProfitFactor = 999
	}
	// Sharpe per-trade: mean/std của pnl từng lệnh
	if len(closedPnL) >= 2 {
		mean := 0.0
		for _, v := range closedPnL {
			mean += v
		}
		mean /= float64(len(closedPnL))
		varV := 0.0
		for _, v := range closedPnL {
			varV += (v - mean) * (v - mean)
		}
		varV /= float64(len(closedPnL) - 1)
		if varV > 0 {
			res.Sharpe = mean / math.Sqrt(varV)
		}
	}
	return res
}

// CorrelationMatrix tính |ρ| giữa các symbol từ close-pct-change chuỗi.
// Trả về map "SYM1|SYM2" → ρ. Dùng cho correlation filter config UI.
func CorrelationMatrix(candles map[string][]exchange.Candle, lookback int) map[string]float64 {
	syms := make([]string, 0, len(candles))
	for s := range candles {
		syms = append(syms, s)
	}
	sort.Strings(syms)
	out := map[string]float64{}
	for i := 0; i < len(syms); i++ {
		for j := i + 1; j < len(syms); j++ {
			a := pctChanges(candles[syms[i]], lookback)
			b := pctChanges(candles[syms[j]], lookback)
			out[syms[i]+"|"+syms[j]] = ta.Pearson(a, b)
		}
	}
	return out
}

func pctChanges(cd []exchange.Candle, lookback int) []float64 {
	if lookback <= 0 || lookback > len(cd)-1 {
		lookback = len(cd) - 1
	}
	if lookback < 1 {
		return nil
	}
	out := make([]float64, 0, lookback)
	start := len(cd) - 1 - lookback
	prev := cd[start].Close
	for i := start + 1; i < len(cd); i++ {
		if prev != 0 {
			out = append(out, (cd[i].Close-prev)/prev*100)
		}
		prev = cd[i].Close
	}
	return out
}

// KeepForFrontend là helper format result cho API (đảm bảo sốserialize sạch).
func (r Result) Summary() string {
	return fmt.Sprintf("%s [%s] trades=%d win=%s%% pnl=%.2f (%.2f%%) mdd=%.2f%% pf=%.2f",
		r.Symbol, r.Strategy, r.Trades, fmt.Sprintf("%.1f", r.WinRate), r.TotalPnL, r.TotalPnLPct, r.MaxDrawdown, r.ProfitFactor)
}
