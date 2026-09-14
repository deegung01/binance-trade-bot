// Package strategy — plug-in strategies (same interface as the Python version).
package strategy

import (
	"fmt"

	"binance-trade-bot/internal/config"
	"binance-trade-bot/internal/exchange"
	"binance-trade-bot/internal/ta"
)

// Signal is an entry signal: side + human reason.
type Signal struct {
	Side   string // "buy"
	Reason string
}

// Strategy computes indicators and entry/exit signals from candles.
type Strategy interface {
	Name() string
	Label() string
	EntrySignal(candles []exchange.Candle) *Signal
	ExitSignal(candles []exchange.Candle, trade *config.Trade) string // "" = no exit
	AdjustSignal(candles []exchange.Candle, trade *config.Trade, cfg config.Config) float64 // extra USDT to add (grid), 0 = none
}

func closes(candles []exchange.Candle) []float64 {
	out := make([]float64, len(candles))
	for i, c := range candles {
		out[i] = c.Close
	}
	return out
}

// ---------------------------------------------------------------------------
// SMA Cross
// ---------------------------------------------------------------------------

type SMACross struct{ Fast, Slow int }

func (s *SMACross) Name() string  { return "sma_cross" }
func (s *SMACross) Label() string { return fmt.Sprintf("SMA Cross (%d/%d)", s.Fast, s.Slow) }

func (s *SMACross) EntrySignal(cd []exchange.Candle) *Signal {
	cl := closes(cd)
	if len(cl) < s.Slow+2 {
		return nil
	}
	fNow, ok1 := ta.SMA(cl, s.Fast)
	sNow, ok2 := ta.SMA(cl, s.Slow)
	fPrev, ok3 := ta.SMA(cl[:len(cl)-1], s.Fast)
	sPrev, ok4 := ta.SMA(cl[:len(cl)-1], s.Slow)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil
	}
	if fPrev <= sPrev && fNow > sNow {
		return &Signal{Side: "buy", Reason: fmt.Sprintf("SMA%d crossed above SMA%d", s.Fast, s.Slow)}
	}
	return nil
}

func (s *SMACross) ExitSignal(cd []exchange.Candle, _ *config.Trade) string {
	cl := closes(cd)
	if len(cl) < s.Slow+2 {
		return ""
	}
	fNow, ok1 := ta.SMA(cl, s.Fast)
	sNow, ok2 := ta.SMA(cl, s.Slow)
	if !ok1 || !ok2 {
		return ""
	}
	if fNow < sNow {
		return "signal"
	}
	return ""
}

func (s *SMACross) AdjustSignal(_ []exchange.Candle, _ *config.Trade, _ config.Config) float64 {
	return 0
}

// ---------------------------------------------------------------------------
// EMA Cross + RSI filter
// ---------------------------------------------------------------------------

type EMACross struct{ Fast, Slow int; RSIMin float64 }

func (s *EMACross) Name() string  { return "ema_cross" }
func (s *EMACross) Label() string { return fmt.Sprintf("EMA Cross (%d/%d) + RSI filter", s.Fast, s.Slow) }

func (s *EMACross) EntrySignal(cd []exchange.Candle) *Signal {
	cl := closes(cd)
	if len(cl) < s.Slow+2 {
		return nil
	}
	fNow, ok1 := ta.EMA(cl, s.Fast)
	sNow, ok2 := ta.EMA(cl, s.Slow)
	fPrev, ok3 := ta.EMA(cl[:len(cl)-1], s.Fast)
	sPrev, ok4 := ta.EMA(cl[:len(cl)-1], s.Slow)
	rsi, ok5 := ta.RSI(cl, 14)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		return nil
	}
	if fPrev <= sPrev && fNow > sNow && rsi > s.RSIMin {
		return &Signal{Side: "buy", Reason: fmt.Sprintf("EMA%d crossed above EMA%d (RSI %.0f)", s.Fast, s.Slow, rsi)}
	}
	return nil
}

func (s *EMACross) ExitSignal(cd []exchange.Candle, _ *config.Trade) string {
	cl := closes(cd)
	if len(cl) < s.Slow+2 {
		return ""
	}
	fNow, ok1 := ta.EMA(cl, s.Fast)
	sNow, ok2 := ta.EMA(cl, s.Slow)
	if !ok1 || !ok2 {
		return ""
	}
	if fNow < sNow {
		return "signal"
	}
	return ""
}

func (s *EMACross) AdjustSignal(_ []exchange.Candle, _ *config.Trade, _ config.Config) float64 {
	return 0
}

// ---------------------------------------------------------------------------
// RSI Reversion
// ---------------------------------------------------------------------------

type RSIRevert struct{ Oversold, Overbought float64 }

func (s *RSIRevert) Name() string  { return "rsi_revert" }
func (s *RSIRevert) Label() string { return fmt.Sprintf("RSI Reversion (%.0f/%.0f)", s.Oversold, s.Overbought) }

func (s *RSIRevert) EntrySignal(cd []exchange.Candle) *Signal {
	cl := closes(cd)
	if len(cl) < 16 {
		return nil
	}
	now, ok1 := ta.RSI(cl, 14)
	prev, ok2 := ta.RSI(cl[:len(cl)-1], 14)
	if !ok1 || !ok2 {
		return nil
	}
	if prev < s.Oversold && now >= s.Oversold {
		return &Signal{Side: "buy", Reason: fmt.Sprintf("RSI crossed up out of oversold (%.0f)", now)}
	}
	return nil
}

func (s *RSIRevert) ExitSignal(cd []exchange.Candle, _ *config.Trade) string {
	cl := closes(cd)
	now, ok := ta.RSI(cl, 14)
	if !ok {
		return ""
	}
	if now >= s.Overbought {
		return "signal"
	}
	return ""
}

func (s *RSIRevert) AdjustSignal(_ []exchange.Candle, _ *config.Trade, _ config.Config) float64 {
	return 0
}

// ---------------------------------------------------------------------------
// MACD histogram flip
// ---------------------------------------------------------------------------

type MACDFlip struct{}

func (s *MACDFlip) Name() string  { return "macd" }
func (s *MACDFlip) Label() string { return "MACD (12/26/9) histogram flip" }

func (s *MACDFlip) EntrySignal(cd []exchange.Candle) *Signal {
	cl := closes(cd)
	_, _, h, ok1 := ta.MACD(cl, 12, 26, 9)
	_, _, hPrev, ok2 := ta.MACD(cl[:len(cl)-1], 12, 26, 9)
	if !ok1 || !ok2 {
		return nil
	}
	if hPrev <= 0 && h > 0 {
		return &Signal{Side: "buy", Reason: fmt.Sprintf("MACD histogram flipped positive (%.4f)", h)}
	}
	return nil
}

func (s *MACDFlip) ExitSignal(cd []exchange.Candle, _ *config.Trade) string {
	cl := closes(cd)
	_, _, h, ok := ta.MACD(cl, 12, 26, 9)
	if !ok {
		return ""
	}
	if h < 0 {
		return "signal"
	}
	return ""
}

func (s *MACDFlip) AdjustSignal(_ []exchange.Candle, _ *config.Trade, _ config.Config) float64 {
	return 0
}

// ---------------------------------------------------------------------------
// Adaptive Grid (ATR spacing + DCA)
// ---------------------------------------------------------------------------

type AdaptiveGrid struct {
	ATRMultiplier     float64
	SpacingMinPct     float64
	SpacingMaxPct     float64
}

func (s *AdaptiveGrid) Name() string  { return "adaptive_grid" }
func (s *AdaptiveGrid) Label() string { return "Adaptive Grid (ATR spacing + DCA)" }

func (s *AdaptiveGrid) spacingPct(cd []exchange.Candle) float64 {
	cl := closes(cd)
	highs := make([]float64, len(cd))
	lows := make([]float64, len(cd))
	for i, c := range cd {
		highs[i], lows[i] = c.High, c.Low
	}
	atr, ok := ta.ATR(highs, lows, cl, 14)
	if !ok || len(cl) == 0 || cl[len(cl)-1] == 0 {
		return 2.0
	}
	raw := atr / cl[len(cl)-1] * 100 * s.ATRMultiplier
	if raw < s.SpacingMinPct {
		return s.SpacingMinPct
	}
	if raw > s.SpacingMaxPct {
		return s.SpacingMaxPct
	}
	return raw
}

func (s *AdaptiveGrid) EntrySignal(cd []exchange.Candle) *Signal {
	cl := closes(cd)
	rsi, ok := ta.RSI(cl, 14)
	if !ok {
		return nil
	}
	if rsi >= 30 && rsi <= 65 {
		return &Signal{Side: "buy", Reason: fmt.Sprintf("grid start: RSI %.0f mid-range, spacing %.2f%%", rsi, s.spacingPct(cd))}
	}
	return nil
}

func (s *AdaptiveGrid) ExitSignal(_ []exchange.Candle, _ *config.Trade) string {
	return "" // exits via SL/TP/trailing, engine-managed
}

func (s *AdaptiveGrid) AdjustSignal(cd []exchange.Candle, trade *config.Trade, cfg config.Config) float64 {
	if trade.Meta == nil {
		return 0
	}
	gridCount := trade.Meta.GridCount
	maxLevels := cfg.GridLevels
	if gridCount >= maxLevels {
		return 0
	}
	if len(cd) == 0 {
		return 0
	}
	price := cd[len(cd)-1].Close
	spacing := s.spacingPct(cd)
	trigger := trade.EntryPrice * (1 - spacing/100)
	if price <= trigger {
		stake := trade.Meta.InitialStake
		if stake <= 0 {
			stake = trade.Stake
		}
		return stake
	}
	return 0
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// All returns the registry of available strategies.
func All() map[string]Strategy {
	return map[string]Strategy{
		"sma_cross":     &SMACross{Fast: 10, Slow: 50},
		"ema_cross":     &EMACross{Fast: 9, Slow: 21, RSIMin: 50},
		"rsi_revert":    &RSIRevert{Oversold: 30, Overbought: 70},
		"macd":          &MACDFlip{},
		"adaptive_grid": &AdaptiveGrid{ATRMultiplier: 1.0, SpacingMinPct: 0.5, SpacingMaxPct: 4.0},
	}
}

// AdaptiveMeta describes the regime engine for the dashboard.
type AdaptiveMeta struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
}

// Get returns a strategy by name (falls back to sma_cross).
func Get(name string) Strategy {
	all := All()
	if name == "adaptive" {
		// regime engine — engine.go xử lý chọn strategy theo regime từng symbol
		return all["ema_cross"]
	}
	if s, ok := all[name]; ok {
		return s
	}
	return all["sma_cross"]
}

// Info describes one strategy for the API.
type Info struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// List returns metadata for the dashboard.
func List() []Info {
	all := All()
	ids := []string{"adaptive", "sma_cross", "ema_cross", "rsi_revert", "macd", "adaptive_grid"}
	out := make([]Info, 0, len(ids))
	for _, id := range ids {
		label := ""
		if id == "adaptive" {
			label = "Adaptive Regime Engine (auto-select per market)"
		} else {
			label = all[id].Label()
		}
		out = append(out, Info{ID: id, Label: label})
	}
	return out
}
