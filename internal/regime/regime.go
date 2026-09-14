// Package regime — adaptive market regime detection.
// Phân loại thị trường mỗi cycle cho từng symbol:
//   trend_up / trend_down / sideways / volatile / quiet
// Dựa trên ADX (độ mạnh trend), ATR ratio (biến động), BB width (dao động),
// EMA slope (hướng). Engine sẽ chọn strategy phù hợp theo regime.
package regime

import (
	"math"

	"binance-trade-bot/internal/exchange"
	"binance-trade-bot/internal/ta"
)

// Regime is a market state classification.
type Regime string

const (
	TrendUp   Regime = "trend_up"
	TrendDown Regime = "trend_down"
	Sideways  Regime = "sideways"
	Volatile  Regime = "volatile"
	Quiet     Regime = "quiet"
)

// Metrics holds the computed values used for classification + dashboard.
type Metrics struct {
	ADX      float64 `json:"adx"`       // trend strength (0–100)
	DIPlus   float64 `json:"di_plus"`   // +DI
	DIMinus  float64 `json:"di_minus"`  // −DI
	ATRRatio float64 `json:"atr_ratio"` // ATR14 / price (%)
	BBWidth  float64 `json:"bb_width"`  // Bollinger bandwidth (%)
	Slope    float64 `json:"slope"`     // EMA50 slope per bar (%)
	Range    float64 `json:"range_pct"` // (max−min)/avg của 50 bar gần nhất (%)
}

// Snapshot is the classification result for one symbol.
type Snapshot struct {
	Symbol    string  `json:"symbol"`
	Regime    Regime  `json:"regime"`
	Metrics   Metrics `json:"metrics"`
	Changed   bool    `json:"changed"`    // regime đổi so với cycle trước (cùng symbol)
	Prev      Regime  `json:"prev"`       // regime trước đó
	Confidence float64 `json:"confidence"` // 0–1
}

// Thresholds — tunable, dùng cả cho dashboard hiển thị.
type Thresholds struct {
	ADXTrend      float64 // ≥ → có trend (mặc định 25)
	ATRHigh       float64 // % — ATR/price ≥ → volatile
	ATRLow        float64 // % — ATR/price < → quiet
	BBWide        float64 // % — BB width ≥ → volatile
	RangePct      float64 // % — range 50 bar; < → sideways
	SlopeStrong   float64 // %/bar — |slope| ≥ → trend
}

func DefaultThresholds() Thresholds {
	return Thresholds{
		ADXTrend:    25,
		ATRHigh:     1.5,
		ATRLow:      0.25,
		BBWide:      5,
		RangePct:    2.5,
		SlopeStrong: 0.02,
	}
}

// Classify phân loại thị trường từ nến.
func Classify(symbol string, candles []exchange.Candle) Snapshot {
	th := DefaultThresholds()
	return ClassifyWith(symbol, candles, th)
}

// ClassifyWith runs classification with explicit thresholds (tests).
func ClassifyWith(symbol string, candles []exchange.Candle, th Thresholds) Snapshot {
	m := Metrics{}
	sn := Snapshot{Symbol: symbol, Prev: ""}
	if len(candles) < 60 {
		sn.Regime = Quiet // không đủ data → mặc định an toàn
		return sn
	}

	highs := make([]float64, len(candles))
	lows := make([]float64, len(candles))
	closes := make([]float64, len(candles))
	for i, c := range candles {
		highs[i], lows[i], closes[i] = c.High, c.Low, c.Close
	}

	adx, dip, dim, ok := ta.ADX(highs, lows, closes, 14)
	if ok {
		m.ADX, m.DIPlus, m.DIMinus = adx, dip, dim
	}
	atr, ok2 := ta.ATR(highs, lows, closes, 14)
	if ok2 && closes[len(closes)-1] > 0 {
		m.ATRRatio = atr / closes[len(closes)-1] * 100
	}
	_, up, low, ok3 := ta.Bollinger(closes, 20, 2)
	if ok3 && low > 0 {
		m.BBWidth = (up - low) / ((up + low) / 2) * 100
	}
	// EMA50 slope: % thay đổi mỗi bar (so sánh 10 bar)
	e1, ok4 := ta.EMA(closes, 50)
	e2, ok5 := ta.EMA(closes[:len(closes)-10], 50)
	if ok4 && ok5 && e2 != 0 {
		m.Slope = (e1 - e2) / e2 / 10 * 100 // % per bar
	}
	// range 50 bar
	mx, mn := 0.0, math.MaxFloat64
	for _, c := range candles[len(candles)-50:] {
		if c.High > mx {
			mx = c.High
		}
		if c.Low < mn {
			mn = c.Low
		}
	}
	if mn > 0 {
		m.Range = (mx - mn) / ((mx + mn) / 2) * 100
	}

	sn.Metrics = m

	// --- phân loại theo thứ tự ưu tiên ---
	// 1) Trend mạnh (ADX + slope + DI): ưu tiên TRƯỚC volatile —
	//    trend mạnh luôn kèm BB rộng, không được để volatile nuốt trend
	if m.ADX >= th.ADXTrend && math.Abs(m.Slope) >= th.SlopeStrong {
		if m.DIPlus > m.DIMinus && m.Slope > 0 {
			sn.Regime = TrendUp
			sn.Confidence = clamp01(m.ADX / 50)
			return sn
		}
		if m.DIMinus > m.DIPlus && m.Slope < 0 {
			sn.Regime = TrendDown
			sn.Confidence = clamp01(m.ADX / 50)
			return sn
		}
	}
	// 2) Volatile: biến động cao, KHÔNG có hướng rõ
	if m.ATRRatio >= th.ATRHigh || m.BBWidth >= th.BBWide {
		sn.Regime = Volatile
		sn.Confidence = clamp01((m.ATRRatio/th.ATRHigh + m.BBWidth/th.BBWide) / 2)
		return sn
	}
	// 3) Quiet: biến động rất thấp
	if m.ATRRatio < th.ATRLow {
		sn.Regime = Quiet
		sn.Confidence = clamp01(1 - m.ATRRatio/th.ATRLow)
		return sn
	}
	// 4) Sideways: range hẹp, không trend
	if m.ADX < th.ADXTrend && m.Range < th.RangePct {
		sn.Regime = Sideways
		sn.Confidence = clamp01((th.ADXTrend-m.ADX)/th.ADXTrend + (th.RangePct-m.Range)/th.RangePct)
	}
	if sn.Regime == "" {
		// ADX yếu nhưng range rộng → vẫn coi sideways (range trading)
		sn.Regime = Sideways
		sn.Confidence = clamp01((th.ADXTrend - m.ADX) / th.ADXTrend)
	}
	return sn
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
