"use client";

import { useEffect, useRef, useState } from "react";
import {
  createChart,
  createSeriesMarkers,
  CrosshairMode,
  LineStyle,
  CandlestickSeries,
} from "lightweight-charts";
import { api, fmtUSD } from "@/lib/api";

const INTERVALS = ["1m", "5m", "15m", "1h", "4h", "1d"];

const fmtQty = (v) =>
  v == null
    ? "—"
    : Number(v).toLocaleString("en-US", { maximumFractionDigits: v >= 100 ? 2 : 6 });

export default function ChartPage() {
  const [symbol, setSymbol] = useState("BTCUSDT");
  // NOTE: không đặt tên state là `interval`/`setInterval` — nó shadow global
  // setInterval và làm hỏng polling (interval trở thành Promise sau tick đầu)
  const [tf, setTf] = useState("5m");
  const [symbols, setSymbols] = useState(["BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"]);
  const [trades, setTrades] = useState([]);
  const [price, setPrice] = useState(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [account, setAccount] = useState(null);
  const chartEl = useRef(null);
  const chartRef = useRef(null);
  const seriesRef = useRef(null);
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

  // load testnet account balances (live mode) — testnet cấp nhiều coin
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
          setErr(`No candles returned for ${symbol}`);
          return;
        }
        seriesRef.current.setData(candles);
        setPrice(candles[candles.length - 1].close);
        setTrades(tr.trades || []);
        // trade markers on this symbol (sorted by time, v5 requirement)
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
  }, [symbol, tf]);

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
            <p className="text-center text-zinc-500 py-4 text-xs">No trades on this symbol yet</p>
          )}
        </div>
      </div>
    </div>
  );
}
