"""
Technical indicators — pure Python, no TA-lib needed.

Standard implementations (same math as pandas-ta / freqtrade):
- SMA, EMA
- RSI (Wilder's smoothing)
- MACD (EMA 12/26, signal 9)
- Bollinger Bands
- ATR (Wilder)
"""
from __future__ import annotations

from typing import List, Optional


def sma(values: List[float], period: int) -> Optional[float]:
    if period <= 0 or len(values) < period:
        return None
    window = values[-period:]
    return sum(window) / period


def ema(values: List[float], period: int) -> Optional[float]:
    if period <= 0 or len(values) < period:
        return None
    k = 2 / (period + 1)
    seed = sum(values[:period]) / period
    e = seed
    for v in values[period:]:
        e = v * k + e * (1 - k)
    return e


def rsi(values: List[float], period: int = 14) -> Optional[float]:
    if len(values) < period + 1:
        return None
    gains, losses = [], []
    for i in range(1, len(values)):
        d = values[i] - values[i - 1]
        gains.append(max(d, 0.0))
        losses.append(max(-d, 0.0))
    # Wilder's smoothing
    avg_gain = sum(gains[:period]) / period
    avg_loss = sum(losses[:period]) / period
    for i in range(period, len(gains)):
        avg_gain = (avg_gain * (period - 1) + gains[i]) / period
        avg_loss = (avg_loss * (period - 1) + losses[i]) / period
    if avg_loss == 0:
        return 100.0
    rs = avg_gain / avg_loss
    return 100 - 100 / (1 + rs)


def macd(values: List[float], fast: int = 12, slow: int = 26, signal: int = 9):
    """Return (macd_line, signal_line, histogram) latest values, or (None,)*3."""
    if len(values) < slow + signal:
        return None, None, None
    k_fast = 2 / (fast + 1)
    k_slow = 2 / (slow + 1)
    k_sig = 2 / (signal + 1)
    ema_fast = sum(values[:fast]) / fast
    ema_slow = sum(values[:slow]) / slow
    macd_line: List[float] = []
    # seed slow EMA after `slow` values; both EMAs must run from same index
    # approximate: compute EMAs series
    ef: List[float] = []
    es: List[float] = []
    e = sum(values[:fast]) / fast
    ef.append(e)
    for v in values[fast:]:
        e = v * k_fast + e * (1 - k_fast)
        ef.append(e)
    e = sum(values[:slow]) / slow
    es.append(e)
    for v in values[slow:]:
        e = v * k_slow + e * (1 - k_slow)
        es.append(e)
    # align tails
    n = min(len(ef), len(es))
    ef2, es2 = ef[-n:], es[-n:]
    macd_line = [a - b for a, b in zip(ef2, es2)]
    if len(macd_line) < signal:
        return None, None, None
    sig = sum(macd_line[:signal]) / signal
    for v in macd_line[signal:]:
        sig = v * k_sig + sig * (1 - k_sig)
    hist = macd_line[-1] - sig
    return macd_line[-1], sig, hist


def bollinger(values: List[float], period: int = 20, mult: float = 2.0):
    if len(values) < period:
        return None, None, None
    window = values[-period:]
    mid = sum(window) / period
    var = sum((x - mid) ** 2 for x in window) / period
    sd = var ** 0.5
    return mid + mult * sd, mid, mid - mult * sd  # upper, mid, lower


def atr(highs: List[float], lows: List[float], closes: List[float], period: int = 14) -> Optional[float]:
    if len(closes) < period + 1:
        return None
    trs = []
    for i in range(1, len(closes)):
        tr = max(
            highs[i] - lows[i],
            abs(highs[i] - closes[i - 1]),
            abs(lows[i] - closes[i - 1]),
        )
        trs.append(tr)
    # Wilder smoothing
    a = sum(trs[:period]) / period
    for i in range(period, len(trs)):
        a = (a * (period - 1) + trs[i]) / period
    return a


def macd_series(values: List[float], fast: int = 12, slow: int = 26, signal: int = 9) -> List[float]:
    """Full histogram series (aligned to input tail) — used for chart panel."""
    n = len(values)
    if n < slow + signal:
        return []
    k_fast = 2 / (fast + 1)
    k_slow = 2 / (slow + 1)
    k_sig = 2 / (signal + 1)
    ef: List[float] = []
    es: List[float] = []
    e = sum(values[:fast]) / fast
    ef.append(e)
    for v in values[fast:]:
        e = v * k_fast + e * (1 - k_fast)
        ef.append(e)
    e = sum(values[:slow]) / slow
    es.append(e)
    for v in values[slow:]:
        e = v * k_slow + e * (1 - k_slow)
    es.append(e)
    es = es[-len(ef):] if len(es) >= len(ef) else es
    # align from the end
    m = min(len(ef), len(es))
    ef2, es2 = ef[-m:], es[-m:]
    macd_line = [a - b for a, b in zip(ef2, es2)]
    if len(macd_line) < signal:
        return []
    sig = sum(macd_line[:signal]) / signal
    out: List[float] = []
    sig_series: List[float] = []
    s = sum(macd_line[:signal]) / signal
    sig_series.append(s)
    for v in macd_line[signal:]:
        s = v * k_sig + s * (1 - k_sig)
        sig_series.append(s)
    hist = [m_ - s_ for m_, s_ in zip(macd_line[-len(sig_series):], sig_series)]
    return hist
