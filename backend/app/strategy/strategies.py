"""
Strategies — plug-in style like freqtrade (each strategy produces entry/exit
signals from candle data).

Interface:
    class Strategy:
        name: str
        def compute(candles) -> dict                 # latest indicator values
        def entry_signal(candles) -> (side, reason)   # ('buy', '...') or None
        def exit_signal(candles, trade) -> str|None  # 'signal' or None
"""
from __future__ import annotations

from typing import List, Optional, Tuple

from app.strategy import indicators as ta


class BaseStrategy:
    name = "base"

    def compute(self, candles: List[dict]) -> dict:
        raise NotImplementedError

    def entry_signal(self, candles: List[dict]) -> Optional[Tuple[str, str]]:
        raise NotImplementedError

    def exit_signal(self, candles: List[dict], trade) -> Optional[str]:
        return None


class SmaCross(BaseStrategy):
    """Golden cross: fast SMA crosses above slow SMA -> buy."""
    name = "sma_cross"
    label = "SMA Cross (10/50)"

    def __init__(self, fast: int = 10, slow: int = 50):
        self.fast = fast
        self.slow = slow

    def compute(self, candles: List[dict]) -> dict:
        closes = [c["close"] for c in candles]
        return {
            "sma_fast": ta.sma(closes, self.fast),
            "sma_slow": ta.sma(closes, self.slow),
            "rsi": ta.rsi(closes, 14),
        }

    def entry_signal(self, candles: List[dict]) -> Optional[Tuple[str, str]]:
        closes = [c["close"] for c in candles]
        if len(closes) < self.slow + 2:
            return None
        f_now = ta.sma(closes, self.fast)
        s_now = ta.sma(closes, self.slow)
        f_prev = ta.sma(closes[:-1], self.fast)
        s_prev = ta.sma(closes[:-1], self.slow)
        if None in (f_now, s_now, f_prev, s_prev):
            return None
        if f_prev <= s_prev and f_now > s_now:
            return ("buy", f"SMA{self.fast} crossed above SMA{self.slow}")
        return None

    def exit_signal(self, candles: List[dict], trade) -> Optional[str]:
        closes = [c["close"] for c in candles]
        if len(closes) < self.slow + 2:
            return None
        f_now = ta.sma(closes, self.fast)
        s_now = ta.sma(closes, self.slow)
        if f_now is None or s_now is None:
            return None
        if f_now < s_now:
            return "signal"
        return None


class EmaCross(BaseStrategy):
    """EMA 9/21 cross with RSI momentum filter."""
    name = "ema_cross"
    label = "EMA Cross (9/21) + RSI filter"

    def __init__(self, fast: int = 9, slow: int = 21, rsi_min: float = 50.0):
        self.fast = fast
        self.slow = slow
        self.rsi_min = rsi_min

    def compute(self, candles: List[dict]) -> dict:
        closes = [c["close"] for c in candles]
        return {
            "ema_fast": ta.ema(closes, self.fast),
            "ema_slow": ta.ema(closes, self.slow),
            "rsi": ta.rsi(closes, 14),
        }

    def entry_signal(self, candles: List[dict]) -> Optional[Tuple[str, str]]:
        closes = [c["close"] for c in candles]
        if len(closes) < self.slow + 2:
            return None
        f_now = ta.ema(closes, self.fast)
        s_now = ta.ema(closes, self.slow)
        f_prev = ta.ema(closes[:-1], self.fast)
        s_prev = ta.ema(closes[:-1], self.slow)
        rsi_now = ta.rsi(closes, 14)
        if None in (f_now, s_now, f_prev, s_prev, rsi_now):
            return None
        if f_prev <= s_prev and f_now > s_now and rsi_now > self.rsi_min:
            return ("buy", f"EMA{self.fast} crossed above EMA{self.slow} (RSI {rsi_now:.0f})")
        return None

    def exit_signal(self, candles: List[dict], trade) -> Optional[str]:
        closes = [c["close"] for c in candles]
        if len(closes) < self.slow + 2:
            return None
        f_now = ta.ema(closes, self.fast)
        s_now = ta.ema(closes, self.slow)
        if f_now is None or s_now is None:
            return None
        if f_now < s_now:
            return "signal"
        return None


class RsiRevert(BaseStrategy):
    """RSI mean reversion: buy when RSI recovers from oversold."""
    name = "rsi_revert"
    label = "RSI Reversion (30/70)"

    def __init__(self, oversold: float = 30.0, overbought: float = 70.0):
        self.oversold = oversold
        self.overbought = overbought

    def compute(self, candles: List[dict]) -> dict:
        closes = [c["close"] for c in candles]
        return {
            "rsi": ta.rsi(closes, 14),
            "sma20": ta.sma(closes, 20),
        }

    def entry_signal(self, candles: List[dict]) -> Optional[Tuple[str, str]]:
        closes = [c["close"] for c in candles]
        if len(closes) < 16:
            return None
        rsi_now = ta.rsi(closes, 14)
        rsi_prev = ta.rsi(closes[:-1], 14)
        if rsi_now is None or rsi_prev is None:
            return None
        if rsi_prev < self.oversold and rsi_now >= self.oversold:
            return ("buy", f"RSI crossed up out of oversold ({rsi_now:.0f})")
        return None

    def exit_signal(self, candles: List[dict], trade) -> Optional[str]:
        closes = [c["close"] for c in candles]
        rsi_now = ta.rsi(closes, 14)
        if rsi_now is None:
            return None
        if rsi_now >= self.overbought:
            return "signal"
        return None


class MacdFlip(BaseStrategy):
    """MACD histogram flips from negative to positive -> buy."""
    name = "macd"
    label = "MACD (12/26/9) histogram flip"

    def compute(self, candles: List[dict]) -> dict:
        closes = [c["close"] for c in candles]
        m, s, h = ta.macd(closes)
        return {"macd": m, "macd_signal": s, "macd_hist": h}

    def entry_signal(self, candles: List[dict]) -> Optional[Tuple[str, str]]:
        closes = [c["close"] for c in candles]
        _, _, h = ta.macd(closes)
        _, _, h_prev = ta.macd(closes[:-1])
        if h is None or h_prev is None:
            return None
        if h_prev <= 0 < h:
            return ("buy", f"MACD histogram flipped positive ({h:.4f})")
        return None

    def exit_signal(self, candles: List[dict], trade) -> Optional[str]:
        closes = [c["close"] for c in candles]
        _, _, h = ta.macd(closes)
        if h is None:
            return None
        if h < 0:
            return "signal"
        return None


STRATEGIES = {
    "sma_cross": SmaCross,
    "ema_cross": EmaCross,
    "rsi_revert": RsiRevert,
    "macd": MacdFlip,
}


def get_strategy(name: str) -> BaseStrategy:
    cls = STRATEGIES.get(name)
    if not cls:
        raise ValueError(f"Unknown strategy: {name}")
    return cls()
