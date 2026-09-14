package regime

import (
	"math"
	"math/rand"
	"testing"

	"binance-trade-bot/internal/exchange"
	"binance-trade-bot/internal/ta"
)

func candle(o, h, l, c float64) exchange.Candle {
	return exchange.Candle{Open: o, High: h, Low: l, Close: c, OpenTime: 0}
}

// genTrendUp: nến uptrend ổn định +1%/5 bar
func genTrendUp(n int) []exchange.Candle {
	out := []exchange.Candle{}
	p := 100.0
	r := rand.New(rand.NewSource(42))
	for i := 0; i < n; i++ {
		next := p * (1 + 0.002 + r.Float64()*0.002)
		out = append(out, candle(p, next*1.001, p*0.999, next))
		p = next
	}
	return out
}

// genTrendDown: downtrend −1%/5 bar
func genTrendDown(n int) []exchange.Candle {
	out := []exchange.Candle{}
	p := 100.0
	r := rand.New(rand.NewSource(42))
	for i := 0; i < n; i++ {
		next := p * (1 - 0.002 - r.Float64()*0.002)
		out = append(out, candle(p, p*1.001, next*0.999, next))
		p = next
	}
	return out
}

// genSideways: dao động ±0.3% quanh 100
func genSideways(n int) []exchange.Candle {
	out := []exchange.Candle{}
	r := rand.New(rand.NewSource(7))
	for i := 0; i < n; i++ {
		drift := (r.Float64() - 0.5) * 0.006
		c := 100 * (1 + drift)
		out = append(out, candle(c, c*1.001, c*0.999, c))
	}
	return out
}

// genVolatile: dao động lớn ±3%/bar
func genVolatile(n int) []exchange.Candle {
	out := []exchange.Candle{}
	r := rand.New(rand.NewSource(99))
	for i := 0; i < n; i++ {
		drift := (r.Float64() - 0.5) * 0.06
		c := 100 * (1 + drift)
		out = append(out, candle(c, c*1.02, c*0.98, c))
	}
	return out
}

// genQuiet: dao động rất nhỏ ±0.05%/bar
func genQuiet(n int) []exchange.Candle {
	out := []exchange.Candle{}
	r := rand.New(rand.NewSource(3))
	for i := 0; i < n; i++ {
		drift := (r.Float64() - 0.5) * 0.001
		c := 100 * (1 + drift)
		out = append(out, candle(c, c*1.0002, c*0.9998, c))
	}
	return out
}

func TestClassifyTrendUp(t *testing.T) {
	sn := Classify("T1", genTrendUp(150))
	if sn.Regime != TrendUp {
		t.Fatalf("want trend_up, got %v (metrics: %+v)", sn.Regime, sn.Metrics)
	}
	if sn.Metrics.ADX < 20 {
		t.Fatalf("ADX too low for a trend: %.1f", sn.Metrics.ADX)
	}
}

func TestClassifyTrendDown(t *testing.T) {
	sn := Classify("T2", genTrendDown(150))
	if sn.Regime != TrendDown {
		t.Fatalf("want trend_down, got %v (metrics: %+v)", sn.Regime, sn.Metrics)
	}
}

func TestClassifySideways(t *testing.T) {
	sn := Classify("T3", genSideways(150))
	if sn.Regime != Sideways {
		t.Fatalf("want sideways, got %v (metrics: %+v)", sn.Regime, sn.Metrics)
	}
}

func TestClassifyVolatile(t *testing.T) {
	sn := Classify("T4", genVolatile(150))
	if sn.Regime != Volatile {
		t.Fatalf("want volatile, got %v (metrics: %+v)", sn.Regime, sn.Metrics)
	}
}

func TestClassifyQuiet(t *testing.T) {
	sn := Classify("T5", genQuiet(150))
	if sn.Regime != Quiet {
		t.Fatalf("want quiet, got %v (metrics: %+v)", sn.Regime, sn.Metrics)
	}
}

// TestShortData: <60 nến → quiet (an toàn)
func TestShortData(t *testing.T) {
	sn := Classify("T6", genSideways(30))
	if sn.Regime != Quiet {
		t.Fatalf("short data should be quiet, got %v", sn.Regime)
	}
}

// TestStrategyFor: mọi regime map sang strategy hợp lệ
func TestStrategyFor(t *testing.T) {
	cases := map[Regime]string{
		TrendUp: "ema_cross", TrendDown: "adaptive_grid", Sideways: "adaptive_grid",
		Volatile: "rsi_revert", Quiet: "sma_cross",
	}
	for r, want := range cases {
		got, _ := StrategyFor(r)
		if got != want {
			t.Fatalf("StrategyFor(%v) = %v, want %v", r, got, want)
		}
	}
}

// TestTransitionSidewaysToTrendUpProfit: quy tắc chính của user
// lệnh sideway có lãi, thị trường chuyển trend up → KHÔNG TP, convert
func TestTransitionSidewaysToTrendUpProfit(t *testing.T) {
	dec := OnTransition(Sideways, TrendUp, true)
	if dec.Action != "convert" {
		t.Fatalf("want convert (hold + trail), got %v (%s)", dec.Action, dec.Reason)
	}
}

// lệnh sideway đang lỗ khi trend up bắt đầu → exit (lag)
func TestTransitionSidewaysToTrendUpLoss(t *testing.T) {
	dec := OnTransition(Sideways, TrendUp, false)
	if dec.Action != "exit" {
		t.Fatalf("want exit, got %v (%s)", dec.Action, dec.Reason)
	}
}

// trend up kết thúc → chốt lời
func TestTransitionTrendUpEnds(t *testing.T) {
	for _, next := range []Regime{Sideways, TrendDown} {
		dec := OnTransition(TrendUp, next, true)
		if dec.Action != "exit" {
			t.Fatalf("trend_up → %v: want exit, got %v", next, dec.Action)
		}
	}
}

// vào volatile đang lãi → convert (trail bám biến động)
func TestTransitionToVolatileProfit(t *testing.T) {
	dec := OnTransition(Sideways, Volatile, true)
	if dec.Action != "convert" {
		t.Fatalf("want convert, got %v", dec.Action)
	}
}

// same regime → hold
func TestTransitionSame(t *testing.T) {
	dec := OnTransition(Sideways, Sideways, true)
	if dec.Action != "hold" {
		t.Fatalf("want hold, got %v", dec.Action)
	}
}

// TestTrailingParams: volatile → trail rộng hơn quiet
func TestTrailingParams(t *testing.T) {
	v := TrailingParams(Volatile, 1.5)
	q := TrailingParams(Quiet, 0.2)
	if v <= q {
		t.Fatalf("volatile trail (%.2f) should be wider than quiet (%.2f)", v, q)
	}
	if v < 0.4 || v > 4 {
		t.Fatalf("trail out of range: %.2f", v)
	}
}

// TestADXKnownValues: chuỗi đơn giản, +DI phải vượt −DI trong uptrend
func TestADXKnownValues(t *testing.T) {
	up := genTrendUp(100)
	highs := make([]float64, len(up))
	lows := make([]float64, len(up))
	closes := make([]float64, len(up))
	for i, c := range up {
		highs[i], lows[i], closes[i] = c.High, c.Low, c.Close
	}
	adx, dip, dim, ok := ta.ADX(highs, lows, closes, 14)
	if !ok {
		t.Fatal("ADX should compute on 100 bars")
	}
	if adx <= 0 || math.IsNaN(adx) {
		t.Fatalf("bad ADX: %v", adx)
	}
	if dip <= dim {
		t.Fatalf("uptrend should have +DI > −DI: %v vs %v", dip, dim)
	}
}

// TestBollingerKnownValues: 20 giá = 10 → mid 10, bands 10±2sd
func TestBollingerKnownValues(t *testing.T) {
	vals := []float64{}
	for i := 0; i < 20; i++ {
		vals = append(vals, 10)
	}
	mid, up, low, ok := ta.Bollinger(vals, 20, 2)
	if !ok || mid != 10 || up != 10 || low != 10 {
		t.Fatalf("flat series: mid=%v up=%v low=%v ok=%v", mid, up, low, ok)
	}
}
