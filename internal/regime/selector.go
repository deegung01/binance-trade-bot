// Adaptive selector: maps regime → strategy + exit behavior, with
// transition rules (sideways trade carried into a new trend).
package regime

// StrategyFor maps a regime to the best-fit strategy id.
//
//	trend_up    → ema_cross   (mua theo momentum, trailing bám sát)
//	trend_down  → adaptive_grid (DCA xuống, rebase avg — chờ đảo)
//	sideways    → adaptive_grid (range trading: mua thấp, TP ở band)
//	volatile     → rsi_revert  (biến động cao: chờ oversold thật sự)
//	quiet        → sma_cross   (breakout chậm, ít noise)
//
// trend_down của spot (long-only) → không entry mới, chỉ quản lý lệnh cũ.
func StrategyFor(r Regime) (id string, label string) {
	switch r {
	case TrendUp:
		return "ema_cross", "Trend Up — EMA Cross + trailing"
	case TrendDown:
		return "adaptive_grid", "Trend Down — Grid DCA (no new entries)"
	case Sideways:
		return "adaptive_grid", "Sideways — Grid range trading"
	case Volatile:
		return "rsi_revert", "Volatile — RSI reversion (deep dips)"
	default:
		return "sma_cross", "Quiet — SMA breakout"
	}
}

// TransitionDecision tells manageTrade how to treat an open sideways/grid
// trade when the regime has just changed.
type TransitionDecision struct {
	Action       string // "hold" | "exit" | "convert"
	Reason       string
}

// OnTransition decides the fate of an open trade when regime changed.
//
// Quy tắc chính (theo yêu cầu): lệnh sideway đang mở, thị trường chuyển
// sang trend CÙNG chiều (long) → không TP ngay, giữ lệnh và chuyển sang
// quản lý theo trend: trailing theo biến động + TP nới rộng.
func OnTransition(old, new Regime, tradeInProfit bool) TransitionDecision {
	// không đổi gì
	if old == new {
		return TransitionDecision{Action: "hold", Reason: "same regime"}
	}
	// sideways → trend_up: lệnh long đang có lãi → convert sang trend mode
	if (old == Sideways || old == Quiet) && new == TrendUp && tradeInProfit {
		return TransitionDecision{
			Action: "convert",
			Reason: "sideways long converted to trend: hold & trail",
		}
	}
	// sideways → trend_up nhưng lệnh đang lỗ → thoát (bắt đầu trend mà lag)
	if (old == Sideways || old == Quiet) && new == TrendUp && !tradeInProfit {
		return TransitionDecision{Action: "exit", Reason: "regime flip: sideways → trend_up, cut lagging trade"}
	}
	// bất kỳ → volatile: nếu đang lãi → trail bám; nếu lỗ → SL chặt hơn
	if new == Volatile {
		if tradeInProfit {
			return TransitionDecision{Action: "convert", Reason: "volatile: switch to volatility trailing"}
		}
		return TransitionDecision{Action: "exit", Reason: "volatile spike against position: cut"}
	}
	// trend_up → sideways/trend_down: trend kết thúc → TP nếu lãi, cắt nếu lỗ
	if old == TrendUp && (new == Sideways || new == TrendDown) {
		if tradeInProfit {
			return TransitionDecision{Action: "exit", Reason: "trend ended: take profit at range top"}
		}
		return TransitionDecision{Action: "exit", Reason: "trend ended against position: cut"}
	}
	// mặc định: trend_down mới → không entry (long-only), giữ lệnh có SL
	return TransitionDecision{Action: "hold", Reason: "regime changed — managing via SL/TP"}
}

// TrailingParams scales the trailing stop distance by volatility.
// Volatility cao → trail rộng hơn (thở được), thấp → trail chặt.
func TrailingParams(r Regime, atrRatio float64) (distancePct float64) {
	base := 1.0 // %
	switch r {
	case TrendUp:
		base = 1.2
	case Sideways, Quiet:
		base = 0.8
	case Volatile:
		base = 2.0
	case TrendDown:
		base = 1.5
	}
	// scale theo ATR ratio: 1.5% ATR ~ baseline
	scaled := base * clamp01(atrRatio/1.5+0.4)
	if scaled < 0.4 {
		scaled = 0.4
	}
	if scaled > 4 {
		scaled = 4
	}
	return scaled
}
