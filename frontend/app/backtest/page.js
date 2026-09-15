"use client";

import { useEffect, useState } from "react";
import { FlaskConical, Play } from "lucide-react";
import { api, fmtUSD } from "@/lib/api";
import { useToast } from "@/components/Toast";

const INTERVALS = ["15m", "30m", "1h", "2h", "4h", "1d"];
const STRATEGIES = [
  { id: "sma_cross", label: "SMA Cross 10/50" },
  { id: "ema_cross", label: "EMA Cross 9/21 + RSI" },
  { id: "rsi_revert", label: "RSI Reversion 30/70" },
  { id: "macd", label: "MACD Flip" },
  { id: "adaptive_grid", label: "Adaptive Grid + DCA" },
];

const inputCls =
  "mt-1 w-full bg-[#0d1117] border border-[#1e2430] rounded-lg px-3 py-2 text-sm text-white outline-none focus:border-amber-400/50";

function Metric({ label, value, tone }) {
  return (
    <td className={`text-right tabular ${tone || "text-zinc-300"}`}>{value}</td>
  );
}

export default function BacktestPage() {
  const toast = useToast();
  const [cfg, setCfg] = useState(null);
  const [symbols, setSymbols] = useState("BTCUSDT,ETHUSDT,SOLUSDT,BNBUSDT");
  const [interval, setInterval_] = useState("1h");
  const [limit, setLimit] = useState(500);
  const [picked, setPicked] = useState(["ema_cross"]);
  const [stake, setStake] = useState(100);
  const [slPct, setSlPct] = useState(2);
  const [tpPct, setTpPct] = useState(4);
  const [trailing, setTrailing] = useState(true);
  const [trailPct, setTrailPct] = useState(1);
  const [cooldownBars, setCooldownBars] = useState(0);
  const [busy, setBusy] = useState(false);
  const [results, setResults] = useState(null);

  useEffect(() => {
    api("/config")
      .then((c) => {
        setCfg(c);
        if (c.trading_symbols) setSymbols(c.trading_symbols);
      })
      .catch((e) => toast.err(e.message));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const toggleStrat = (id) => {
    setPicked((prev) =>
      prev.includes(id) ? prev.filter((s) => s !== id) : [...prev, id]
    );
  };

  const run = async () => {
    if (picked.length === 0) {
      toast.err("Chọn ít nhất 1 strategy");
      return;
    }
    setBusy(true);
    setResults(null);
    try {
      const res = await api("/backtest", {
        method: "POST",
        body: JSON.stringify({
          symbols,
          interval,
          limit: Number(limit),
          strategies: picked,
          start_balance: 10000,
          stake: Number(stake),
          sl_pct: Number(slPct),
          tp_pct: Number(tpPct),
          trailing,
          trail_pct: Number(trailPct),
          cooldown_bars: Number(cooldownBars),
        }),
      });
      setResults(res.results);
      const ok = res.results.filter((r) => r.result).length;
      toast.ok(`Backtest xong: ${ok} tổ hợp symbol × strategy`);
    } catch (e) {
      toast.err(e.message);
    }
    setBusy(false);
  };

  const rows = results || [];

  return (
    <div className="space-y-4 max-w-[1400px]">
      <div>
        <h1 className="text-xl font-semibold text-white">Backtest</h1>
        <p className="text-sm text-zinc-500">
          Chạy strategy trên nến lịch sử (không look-ahead) — đo win rate, PnL, drawdown, profit factor
        </p>
      </div>

      {/* params */}
      <div className="panel p-5">
        <div className="grid md:grid-cols-3 lg:grid-cols-4 gap-4">
          <div className="md:col-span-2">
            <label className="text-xs text-zinc-500">Symbols (phẩy cách nhau)</label>
            <input value={symbols} onChange={(e) => setSymbols(e.target.value.toUpperCase())} className={inputCls} />
          </div>
          <div>
            <label className="text-xs text-zinc-500">Khung thời gian</label>
            <select value={interval} onChange={(e) => setInterval_(e.target.value)} className={inputCls}>
              {INTERVALS.map((i) => (
                <option key={i}>{i}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="text-xs text-zinc-500">Số nến</label>
            <select value={limit} onChange={(e) => setLimit(Number(e.target.value))} className={inputCls}>
              {[300, 500, 800, 1000].map((n) => (
                <option key={n} value={n}>{n}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="text-xs text-zinc-500">Stake (USDT)</label>
            <input type="number" value={stake} onChange={(e) => setStake(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="text-xs text-zinc-500">Stop loss %</label>
            <input type="number" step="0.1" value={slPct} onChange={(e) => setSlPct(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="text-xs text-zinc-500">Take profit %</label>
            <input type="number" step="0.1" value={tpPct} onChange={(e) => setTpPct(e.target.value)} className={inputCls} />
          </div>
          <div className="flex items-end gap-2">
            <div className="flex-1">
              <label className="text-xs text-zinc-500">Trailing %</label>
              <input
                type="number" step="0.1" value={trailPct} disabled={!trailing}
                onChange={(e) => setTrailPct(e.target.value)}
                className={`${inputCls} ${!trailing ? "opacity-40" : ""}`}
              />
            </div>
            <button
              onClick={() => setTrailing(!trailing)}
              className={`mb-[2px] px-3 py-2 rounded-lg text-xs ${
                trailing ? "bg-amber-400/15 text-amber-300" : "bg-zinc-500/10 text-zinc-500"
              }`}
            >
              {trailing ? "ON" : "OFF"}
            </button>
          </div>
          <div>
            <label className="text-xs text-zinc-500">Cooldown (nến)</label>
            <input type="number" min="0" value={cooldownBars} onChange={(e) => setCooldownBars(e.target.value)} className={inputCls} />
          </div>
        </div>

        {/* strategy multi-select */}
        <div className="mt-4">
          <label className="text-xs text-zinc-500">Strategies</label>
          <div className="mt-1.5 flex flex-wrap gap-2">
            {STRATEGIES.map((s) => (
              <button
                key={s.id}
                onClick={() => toggleStrat(s.id)}
                className={`text-xs px-3 py-1.5 rounded-lg border transition-colors ${
                  picked.includes(s.id)
                    ? "border-amber-400/40 bg-amber-400/10 text-amber-300"
                    : "border-[#1e2430] text-zinc-500 hover:text-white"
                }`}
              >
                {s.label}
              </button>
            ))}
          </div>
        </div>

        <button
          onClick={run}
          disabled={busy}
          className="mt-5 px-5 py-2.5 rounded-lg bg-sky-500/15 text-sky-300 hover:bg-sky-500/25 text-sm font-medium flex items-center gap-2"
        >
          <Play size={14} className={busy ? "animate-pulse" : ""} />
          {busy ? "Đang chạy…" : "Chạy backtest"}
        </button>
      </div>

      {/* results */}
      {rows.length > 0 && (
        <div className="panel overflow-x-auto">
          <table className="w-full text-xs">
            <thead>
              <tr className="text-zinc-500 border-b border-[#1e2430]">
                <th className="text-left py-3 px-4 font-normal">Symbol</th>
                <th className="text-left font-normal">Strategy</th>
                <th className="text-right font-normal">Trades</th>
                <th className="text-right font-normal">Win rate</th>
                <th className="text-right font-normal">PnL</th>
                <th className="text-right font-normal">PnL %</th>
                <th className="text-right font-normal">Max DD</th>
                <th className="text-right font-normal">PF</th>
                <th className="text-right font-normal">Sharpe</th>
                <th className="text-right font-normal pr-4">Expectancy</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => {
                if (r.error) {
                  return (
                    <tr key={i} className="border-b border-[#161c28]">
                      <td className="py-2 px-4 text-white">{r.symbol}</td>
                      <td colSpan={9} className="text-rose-400">{r.error}</td>
                    </tr>
                  );
                }
                const d = r.result;
                const up = d.total_pnl >= 0;
                return (
                  <tr key={i} className="border-b border-[#161c28] hover:bg-white/[0.02]">
                    <td className="py-2 px-4 text-white font-medium">{r.symbol}</td>
                    <td className="text-zinc-400">{r.strategy}</td>
                    <Metric label="trades" value={d.trades} />
                    <Metric
                      label="win"
                      value={`${d.win_rate.toFixed(1)}%`}
                      tone={d.win_rate >= 50 ? "text-emerald-400" : "text-zinc-300"}
                    />
                    <Metric
                      label="pnl"
                      value={`${up ? "+" : ""}${fmtUSD(d.total_pnl)}`}
                      tone={up ? "text-emerald-400" : "text-rose-400"}
                    />
                    <Metric
                      label="pnl_pct"
                      value={`${d.total_pnl_pct >= 0 ? "+" : ""}${d.total_pnl_pct.toFixed(2)}%`}
                      tone={up ? "text-emerald-400" : "text-rose-400"}
                    />
                    <Metric label="mdd" value={`${d.max_drawdown.toFixed(2)}%`} tone="text-amber-400/80" />
                    <Metric
                      label="pf"
                      value={d.profit_factor >= 999 ? "∞" : d.profit_factor.toFixed(2)}
                      tone={d.profit_factor >= 1 ? "text-emerald-400" : "text-rose-400"}
                    />
                    <Metric label="sharpe" value={d.sharpe ? d.sharpe.toFixed(2) : "—"} />
                    <Metric
                      label="exp"
                      value={`${d.expectancy >= 0 ? "+" : ""}${fmtUSD(d.expectancy)}`}
                      tone={d.expectancy >= 0 ? "text-emerald-400" : "text-rose-400"}
                    />
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {!results && !busy && (
        <div className="panel p-10 text-center text-sm text-zinc-500">
          <FlaskConical size={28} className="mx-auto mb-3 text-zinc-600" />
          Cấu hình tham số ở trên rồi nhấn <b className="text-zinc-300">Chạy backtest</b>.
          Kết quả: trades, win rate, PnL, max drawdown, profit factor, Sharpe — từng symbol × strategy.
        </div>
      )}
    </div>
  );
}
