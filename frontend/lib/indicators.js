// Client-side technical indicators (same math as Go internal/ta).

export function sma(values, period) {
  if (values.length < period) return null;
  let sum = 0;
  for (let i = values.length - period; i < values.length; i++) sum += values[i];
  return sum / period;
}

// EMA series cho toàn bộ mảng (để vẽ line overlay).
export function emaSeries(values, period) {
  if (values.length < period) return [];
  const k = 2 / (period + 1);
  const out = [];
  let e = 0;
  for (let i = 0; i < period; i++) e += values[i];
  e /= period;
  out.push(e);
  for (let i = period; i < values.length; i++) {
    e = values[i] * k + e * (1 - k);
    out.push(e);
  }
  return out;
}

export function smaSeries(values, period) {
  if (values.length < period) return [];
  const out = [];
  for (let i = period; i <= values.length; i++) {
    let sum = 0;
    for (let j = i - period; j < i; j++) sum += values[j];
    out.push(sum / period);
  }
  return out;
}

// Bollinger (20, 2) — trả về 2 series (upper, lower).
export function bbSeries(values, period = 20, mult = 2) {
  if (values.length < period) return { upper: [], lower: [] };
  const upper = [];
  const lower = [];
  for (let i = period; i <= values.length; i++) {
    const win = values.slice(i - period, i);
    const mean = win.reduce((a, b) => a + b, 0) / period;
    const variance = win.reduce((a, b) => a + (b - mean) ** 2, 0) / period;
    const sd = Math.sqrt(variance);
    upper.push(mean + mult * sd);
    lower.push(mean - mult * sd);
  }
  return { upper, lower };
}
