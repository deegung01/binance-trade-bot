"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  createChart,
  createSeriesMarkers,
  CrosshairMode,
  CandlestickSeries,
  HistogramSeries,
  LineSeries,
} from "lightweight-charts";
import { api, fmtUSD } from "@/lib/api";
import { emaSeries, smaSeries, bbSeries } from "@/lib/indicators";
import { useToast } from "@/components/Toast";

const INTERVALS = ["1m", "5m", "15m", "1h", "4h", "1d"];

const fmtQty = (v) =>
  v == null
    ? "—"
    : Number(v).toLocaleString("en-US", { maximumFractionDigits: v >= 100 ? 2 : 6 });

const OVERLAYS = [
  { id: "ema", label: "EMA 9/21", color1: "#38bdf8", color2: "#f472b6" },
  { id: "sma", label: "SMA 10/50", color1: "#facc15", color2: "#a78bfa" },
  { id: "bb", label: "Bollinger", color1: "#64748b", color2: "#64748b" },
];

export default function ChartPage() {
  const toast = useToast();
  const [symbol, setSymbol] = useState("BTCUSDT");
  // NOTE: không đặt tên state là `interval` — shadow global setInterval
  const [tf, setTf] = useState("5m");
  const [symbols, setSymbols] = useState(["BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"]);
  const [trades, setTrades] = useState([]);
  const [price, setPrice] = useState(null);
  const [err, setErr] = useState("");
  const [account, setAccount] = useState(null);
  const [on, setOn] = useState({ ema: true, sma: false, bb: false });
  const [rawCandles, setRawCandles] = useState([]);
  const chartEl = useRef(null);
  const chartRef = useRef(null);
  const seriesRef = useRef(null);
  const volumeRef = useRef(null);
  const overlayRefs = useRef({}); // key → series
  const markersRef = useRef(null);

  // init chart once (lightweight-charts v5 API)
  useEffect(() => {
    const chart = createChart(chartEl.current, {
      layout: {
        background: { type: "solid", color: "transparent" },
        textColor: "#71767f",
        fontSize: 11,
      },
      grid: {
        vertLines: { color: "#161c28" },
        horzLines: { color: "#161c28" },
      },
      crosshair: {
        mode: CrosshairMode.Normal,
        vertLine: { color: "#3f4757", labelBackgroundColor: "#1e2430" },
        horzLine: { color: "#3f4757", labelBackgroundColor: "#1e2430" },
      },
      rightPriceScale: { borderColor: "#1e2430" },
      timeScale: { borderColor: "#1e2430", timeVisible: true, secondsVisible: false },
      autoSize: true,
    });
    chartRef.current = chart;
    const series = chart.addSeries(CandlestickSeries, {
      upColor: "#16b981",
      downColor: "#f43f5e",
      wickUpColor: "#16b981",
      wickDownColor: "#f43f5e",
      borderVisible: false,
    });
    seriesRef.current = series;
    const vol = chart.addSeries(HistogramSeries, {
      priceFormat: { type: "volume" },
      priceScaleId: "vol",
    });
    chart.priceScale("vol").applyOptions({ scaleMargins: { top: 0.85, bottom: 0 } });
    volumeRef.current = vol;
    markersRef.current = createSeriesMarkers(series, []);
    return () => chart.remove();
  }, []);

  // load symbol list once
  useEffect(() => {
    api("/symbols")
      .then((d) => setSymbols((prev) => {
        const set = new Set([...prev, ...(d.symbols || []).slice(0, 200)]);
        return [...set];
      }))
      .catch(() => {});
  }, []);

  // load testnet account balances (live mode)
  useEffect(() => {
    let alive = true;
    const loadAcc = async () => {
      try {
        const a = await api("/account");
        if (alive) setAccount(a);
      } catch {}
    };
    loadAcc();
    const t = setInterval(loadAcc, 20000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  // load candles + trades
  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const [c, tr] = await Promise.all([
          api(`/candles?symbol=${symbol}&interval=${tf}&limit=300`),
          api(`/trades?limit=200`),
        ]);
        if (!alive) return;
        const candles = (c.candles || []).map((k) => ({
          time: Math.floor(k.open_time / 1000),
          open: k.open,
          high: k.high,
          low: k.low,
          close: k.close,
        }));
        if (candles.length === 0) {
          setErr(`Không có nến cho ${symbol}`);
          return;
        }
        seriesRef.current.setData(candles);
        setPrice(candles[candles.length - 1].close);
        setRawCandles(c.candles || []);
        // volume histogram
        const vols = (c.candles || []).map((k) => ({
          time: Math.floor(k.open_time / 1000),
          value: k.volume,
          color: k.close >= k.open ? "rgba(22,185,129,0.35)" : "rgba(244,63,94,0.35)",
        }));
        volumeRef.current.setData(vols);
        setTrades(tr.trades || []);
        const marks = tr.trades
          .filter((t) => t.symbol === symbol)
          .map((t) => ({
            time: Math.floor(new Date(t.opened_at).getTime() / 1000),
            position: "belowBar",
            color: "#38bdf8",
            shape: "arrowUp",
            text: `BUY ${fmtUSD(t.entry_price)}`,
          }))
          .sort((a, b) => a.time - b.time);
        markersRef.current.setMarkers(marks);
        setErr("");
      } catch (e) {
        setErr(e.message);
      }
    };
    load();
    const t = setInterval(load, 15000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, [symbol, tf]);

  // overlay indicators — vẽ lại khi candles hoặc toggle đổi
  useEffect(() => {
    const chart = chartRef.current;
    if (!chart || rawCandles.length === 0) return;
    const times = rawCandles.map((k) => Math.floor(k.open_time / 1000));
    const closes = rawCandles.map((k) => k.close);

    // xóa overlay cũ
    for (const key of Object.keys(overlayRefs.current)) {
      try { chart.removeSeries(overlayRefs.current[key]); } catch {}
      delete overlayRefs.current[key];
    }

    const draw = (key, values, color, width = 1.4) => {
      const offset = times.length - values.length;
      const data = values
        .map((v, i) => ({ time: times[i + offset], value: v }))
        .filter((d) => d.time != null);
      const s = chart.addSeries(LineSeries, {
        color,
        lineWidth: width,
        priceLineVisible: false,
        lastValueVisible: false,
        crosshairMarkerVisible: false,
      });
      s.setData(data);
      overlayRefs.current[key] = s;
    };

    if (on.ema) {
      const e9 = emaSeries(closes, 9);
      const e21 = emaSeries(closes, 21);
      draw("ema9", e9, OVERLAYS[0].color1, 1.5);
      draw("ema21", e21, OVERLAYS[0].color2, 1.5);
    }
    if (on.sma) {
      const s10 = smaSeries(closes, 10);
      const s50 = smaSeries(closes, 50);
      draw("sma10", s10, OVERLAYS[1].color1, 1.5);
      draw("sma50", s50, OVERLAYS[1].color2, 1.5);
    }
    if (on.bb) {
      const { upper, lower } = bbSeries(closes, 20, 2);
      draw("bbu", upper, OVERLAYS[2].color1, 1);
      draw("bbl", lower, OVERLAYS[2].color2, 1);
    }
  }, [rawCandles, on]);

  return (
    <div className="space-y-4 max-w-[1400px]">
      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-xl font-semibold text-white">Chart</h1>
        <select
          value={symbol}
          onChange={(e) => setSymbol(e.target.value)}
          className="bg-[#11151d] border border-[#1e2430] rounded-lg px-3 py-1.5 text-sm text-white outline-none focus:border-amber-400/50"
        >
          {symbols.map((s) => (
            <option key={s}>{s}</option>
          ))}
        </select>
        <div className="flex rounded-lg overflow-hidden border border-[#1e2430]">
          {INTERVALS.map((i) => (
            <button
              key={i}
              onClick={() => setTf(i)}
              className={`px-3 py-1.5 text-xs ${
                tf === i ? "bg-amber-400/15 text-amber-300" : "text-zinc-400 hover:text-white"
              }`}
            >
              {i}
            </button>
          ))}
        </div>
        {price != null && (
          <span className="text-lg font-semibold text-white tabular">{fmtUSD(price)}</span>
        )}
        {err && <span className="text-xs text-rose-400">{err}</span>}
      </div>

      {/* indicator toggles */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[11px] text-zinc-600">Indicators:</span>
        {OVERLAYS.map((o) => (
          <button
            key={o.id}
            onClick={() => setOn((prev) => ({ ...prev, [o.id]: !prev[o.id] }))}
            className={`text-[11px] px-2.5 py-1 rounded-lg border flex items-center gap-1.5 transition-colors ${
              on[o.id]
                ? "border-amber-400/40 bg-amber-400/10 text-amber-300"
                : "border-[#1e2430] text-zinc-500 hover:text-white"
            }`}
          >
            <span
              className="inline-block w-2 h-2 rounded-full"
              style={{ background: on[o.id] ? o.color1 : "#3f4757" }}
            />
            {o.label}
          </button>
        ))}
      </div>

      {/* testnet balances (live mode) */}
      {account?.live && account.balances?.length > 0 && (
        <div className="panel p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-medium text-white">
              Testnet account <span className="text-emerald-400 text-xs ml-1">LIVE</span>
            </h2>
            <span className="text-sm font-semibold text-white tabular">
              {fmtUSD(account.total_usdt)}{" "}
              <span className="text-xs text-zinc-500 font-normal">total ({account.balances.length} coins)</span>
            </span>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-zinc-500 border-b border-[#1e2430]">
                  <th className="text-left py-2 font-normal">Asset</th>
                  <th className="text-right font-normal">Free</th>
                  <th className="text-right font-normal">Locked</th>
                  <th className="text-right font-normal">Price (USDT)</th>
                  <th className="text-right font-normal">Value (USDT)</th>
                  <th className="text-right font-normal pr-2">%</th>
                </tr>
              </thead>
              <tbody>
                {account.balances.map((b) => {
                  const total = account.total_usdt || 0;
                  const pct = total > 0 ? (b.usdt_value / total) * 100 : 0;
                  return (
                    <tr key={b.asset} className="border-b border-[#161c28] hover:bg-white/[0.02]">
                      <td className="py-1.5 text-white font-medium">{b.asset}</td>
                      <td className="text-right tabular text-zinc-300">{fmtQty(b.free)}</td>
                      <td className="text-right tabular text-zinc-500">{b.locked > 0 ? fmtQty(b.locked) : "—"}</td>
                      <td className="text-right tabular text-zinc-400">{b.price ? fmtUSD(b.price, b.price >= 100 ? 2 : 4) : "—"}</td>
                      <td className="text-right tabular text-zinc-300">{fmtUSD(b.usdt_value)}</td>
                      <td className="text-right tabular text-zinc-500 pr-2">{pct.toFixed(1)}%</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <div className="panel p-2">
        <div ref={chartEl} className="h-[480px] w-full" />
      </div>

      <div className="panel p-4">
        <h2 className="text-sm font-medium text-white mb-3">
          Trades on {symbol} <span className="text-zinc-500">({trades.filter((t) => t.symbol === symbol).length})</span>
        </h2>
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="text-zinc-500 border-b border-[#1e2430]">
                <th className="text-left py-2 font-normal">Time</th>
                <th className="text-left font-normal">Side</th>
                <th className="text-right font-normal">Qty</th>
                <th className="text-right font-normal">Entry</th>
                <th className="text-right font-normal">Exit</th>
                <th className="text-right font-normal">PnL</th>
                <th className="text-right font-normal">Status</th>
              </tr>
            </thead>
            <tbody>
              {trades
                .filter((t) => t.symbol === symbol)
                .map((t) => (
                  <tr key={t.id} className="border-b border-[#161c28]">
                    <td className="py-1.5 text-zinc-400">
                      {new Date(t.opened_at).toLocaleString("en-GB", {
                        day: "2-digit",
                        month: "short",
                        hour: "2-digit",
                        minute: "2-digit",
                      })}
                    </td>
                    <td className="text-sky-400">BUY</td>
                    <td className="text-right tabular text-zinc-300">{Number(t.qty).toFixed(6)}</td>
                    <td className="text-right tabular text-zinc-300">{fmtUSD(t.entry_price)}</td>
                    <td className="text-right tabular text-zinc-300">{t.exit_price ? fmtUSD(t.exit_price) : "—"}</td>
                    <td className={`text-right tabular ${t.pnl > 0 ? "text-emerald-400" : t.pnl < 0 ? "text-rose-400" : "text-zinc-500"}`}>
                      {t.status === "closed" ? `${t.pnl > 0 ? "+" : ""}${fmtUSD(t.pnl)}` : "—"}
                    </td>
                    <td className="text-right">
                      <span className={`px-1.5 py-0.5 rounded text-[10px] ${t.status === "open" ? "bg-sky-500/15 text-sky-300" : "bg-zinc-500/15 text-zinc-400"}`}>
                        {t.status}
                      </span>
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
          {trades.filter((t) => t.symbol === symbol).length === 0 && (
            <p className="text-center text-zinc-500 py-4 text-xs">Chưa có lệnh nào trên symbol này</p>
          )}
        </div>
      </div>
    </div>
  );
}
