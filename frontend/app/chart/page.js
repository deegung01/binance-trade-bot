"use client";

import { useEffect, useRef, useState } from "react";
import {
  createChart,
  CrosshairMode,
  LineStyle,
} from "lightweight-charts";
import { api, fmtUSD } from "@/lib/api";

const INTERVALS = ["1m", "5m", "15m", "1h", "4h", "1d"];

export default function ChartPage() {
  const [symbol, setSymbol] = useState("BTCUSDT");
  const [interval, setInterval] = useState("5m");
  const [symbols, setSymbols] = useState(["BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"]);
  const [trades, setTrades] = useState([]);
  const [price, setPrice] = useState(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const chartEl = useRef(null);
  const chartRef = useRef(null);
  const seriesRef = useRef(null);

  // init chart once
  useEffect(() => {
    const chart = createChart(chartEl.current, {
      layout: {
        background: { color: "transparent" },
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
    const series = chart.addCandlestickSeries({
      upColor: "#16b981",
      downColor: "#f43f5e",
      wickUpColor: "#16b981",
      wickDownColor: "#f43f5e",
      borderVisible: false,
    });
    seriesRef.current = series;
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

  // load candles + trades
  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const [c, tr] = await Promise.all([
          api(`/candles?symbol=${symbol}&interval=${interval}&limit=300`),
          api(`/trades?limit=200`),
        ]);
        if (!alive) return;
        const candles = c.candles.map((k) => ({
          time: Math.floor(k.open_time / 1000),
          open: k.open,
          high: k.high,
          low: k.low,
          close: k.close,
        }));
        seriesRef.current.setData(candles);
        setPrice(candles[candles.length - 1].close);
        setTrades(tr.trades);
        // trade markers on this symbol
        const marks = tr.trades
          .filter((t) => t.symbol === symbol)
          .map((t) => ({
            time: Math.floor(new Date(t.opened_at).getTime() / 1000),
            position: "belowBar",
            color: "#38bdf8",
            shape: "arrowUp",
            text: `BUY ${fmtUSD(t.entry_price)}`,
          }));
        seriesRef.current.setMarkers(marks);
        setErr("");
      } catch (e) {
        setErr(e.message);
      } finally {
        setLoading(false);
      }
    };
    load();
    const t = setInterval(load, 15000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, [symbol, interval]);

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
              onClick={() => setInterval(i)}
              className={`px-3 py-1.5 text-xs ${
                interval === i ? "bg-amber-400/15 text-amber-300" : "text-zinc-400 hover:text-white"
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
            <p className="text-center text-zinc-500 py-4 text-xs">No trades on this symbol yet</p>
          )}
        </div>
      </div>
    </div>
  );
}
