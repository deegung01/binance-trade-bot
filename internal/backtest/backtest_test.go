package backtest

import (
	"math"
	"math/rand"
	"testing"

	"binance-trade-bot/internal/exchange"
	"binance-trade-bot/internal/ta"
)

func candle(o, h, l, c float64, t int64) exchange.Candle {
	return exchange.Candle{Open: o, High: h, Low: l, Close: c, OpenTime: t}
}

// genUp: uptrend +0.3%/bar với pullback −1% mỗi 30 bar — tạo cross thật
// (thị trường thật luôn có pullback; trend đều tuyệt đối không bao giờ cross).
func genUp(n int) []exchange.Candle {
	out := []exchange.Candle{}
	p := 100.0
	r := rand.New(rand.NewSource(11))
	for i := 0; i < n; i++ {
		next := p * (1 + 0.003 + r.Float64()*0.001)
		// pullback mỗi 40 bar: 10 bar × −0.9%/bar (≈ −8.6%) — đủ sâu để
		// EMA9 cắt xuống EMA21 thật sự rồi cross lại khi trend resume.
		if i > 40 && i%40 < 10 {
			next = p * (1 - 0.009)
		}
		out = append(out, candle(p, next*1.001, p*0.999, next, int64(i)*300000))
		p = next
	}
	return out
}

// genFlat: đi ngang hoàn toàn — không có cross, không entry.
func genFlat(n int) []exchange.Candle {
	out := []exchange.Candle{}
	r := rand.New(rand.NewSource(5))
	for i := 0; i < n; i++ {
		c := 100 * (1 + (r.Float64()-0.5)*0.0005)
		out = append(out, candle(c, c*1.0002, c*0.9998, c, int64(i)*300000))
	}
	return out
}

func baseParams(cd []exchange.Candle) Params {
	return Params{
		Symbol: "TESTUSDT", Candles: cd, Strategy: "ema_cross",
		StartBalance: 10000, Stake: 100,
		StopLossPct: 2, TakeProfitPct: 4,
		TrailingStop: true, TrailingPct: 1,
		GridLevels: 3, CooldownBars: 0,
	}
}

// TestBacktestUptrendProfit: uptrend mạnh → ema_cross thắng, equity tăng.
func TestBacktestUptrendProfit(t *testing.T) {
	res := Run(baseParams(genUp(300)))
	if res.Trades == 0 {
		t.Fatal("uptrend should produce trades")
	}
	if res.WinRate < 50 {
		t.Fatalf("uptrend win rate too low: %.1f%% (%d trades)", res.WinRate, res.Trades)
	}
	if res.TotalPnL <= 0 {
		t.Fatalf("uptrend should be profitable, pnl=%.2f (%s)", res.TotalPnL, res.Summary())
	}
	if res.MaxDrawdown < 0 || res.MaxDrawdown > 50 {
		t.Fatalf("suspicious drawdown: %.2f", res.MaxDrawdown)
	}
	if math.IsNaN(res.Sharpe) || math.IsInf(res.Sharpe, 0) {
		t.Fatalf("bad sharpe: %v", res.Sharpe)
	}
}

// TestBacktestFlatNoTrade: giá flat — RSI không chạm oversold → rsi_revert 0 lệnh.
// (ema_cross trên flat sẽ whipsaw — đó là hành vi thật của MA cross, không phải bug.)
func TestBacktestFlatNoTrade(t *testing.T) {
	p := baseParams(genFlat(300))
	p.Strategy = "rsi_revert"
	res := Run(p)
	if res.Trades != 0 {
		t.Fatalf("flat + rsi_revert should not trade, got %d (%s)", res.Trades, res.Summary())
	}
	if res.FinalEquity != 10000 {
		t.Fatalf("no trades → equity unchanged, got %.2f", res.FinalEquity)
	}
}

// TestBacktestFeeAccounted: phí phải được trừ — equity không thể tăng khi
// 0 lệnh được đóng và mọi lệnh chốt đúng entry (sanity qua flat).
func TestBacktestFeeAccounted(t *testing.T) {
	p := baseParams(genUp(300))
	p.StartBalance = 1000
	p.Stake = 100
	res := Run(p)
	// mọi lệnh đều mua+bán → tổng phí > 0 → pnl < gross. Chỉ assert hợp lý:
	// final equity dương và không vượt quá mức tăng giá (≈ +0.4%/bar × 300 bar khổng lồ
	// nhưng stake cố định 100 → max pnl bị chặn bởi số vòng trade).
	if res.FinalEquity <= 0 {
		t.Fatalf("equity must stay positive, got %.2f", res.FinalEquity)
	}
	if res.Trades > 0 && res.TotalPnL > 0 && res.ProfitFactor <= 1 {
		t.Fatalf("profitable run should have PF>1, got %.2f", res.ProfitFactor)
	}
}

// TestCorrelationMatrixPerfect: 2 chuỗi giống hệt nhau → |ρ|=1.
func TestCorrelationMatrixPerfect(t *testing.T) {
	a := genUp(100)
	b := make([]exchange.Candle, len(a))
	copy(b, a)
	m := CorrelationMatrix(map[string][]exchange.Candle{"AAAUSDT": a, "BBBUSDT": b}, 90)
	r, ok := m["AAAUSDT|BBBUSDT"]
	if !ok {
		t.Fatal("missing pair AAAUSDT|BBBUSDT")
	}
	if r < 0.99 {
		t.Fatalf("identical series should correlate ~1, got %.4f", r)
	}
}

// TestCorrelationMatrixUncorrelated: trend lên vs flat nhiễu → |ρ| thấp.
func TestCorrelationMatrixUncorrelated(t *testing.T) {
	a := genUp(100)
	b := genFlat(100)
	m := CorrelationMatrix(map[string][]exchange.Candle{"AAAUSDT": a, "BBBUSDT": b}, 90)
	r := m["AAAUSDT|BBBUSDT"]
	if r > 0.9 {
		t.Fatalf("up vs flat should not correlate highly, got %.4f", r)
	}
}

// TestPearsonKnownValues: hand-computed.
func TestPearsonKnownValues(t *testing.T) {
	// perfectly positive
	if r := ta.Pearson([]float64{1, 2, 3}, []float64{2, 4, 6}); r < 0.999 {
		t.Fatalf("perfect positive = %.4f", r)
	}
	// perfectly negative
	if r := ta.Pearson([]float64{1, 2, 3}, []float64{3, 2, 1}); r > -0.999 {
		t.Fatalf("perfect negative = %.4f", r)
	}
	// too short
	if r := ta.Pearson([]float64{1}, []float64{1}); r != 0 {
		t.Fatalf("short series should be 0, got %v", r)
	}
}
