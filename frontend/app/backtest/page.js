"use client";

import { useEffect, useMemo, useState } from "react";
import { FlaskConical, Play, Download } from "lucide-react";
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

const inputCls = "input";
const selectCls = "select";

function Metric({ label, value, tone }) {
  return <td className={`text-right tabular ${tone || "text-zinc-300"}`}>{value}</td>;
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
      .then((c) => { setCfg(c); if (c.trading_symbols) setSymbols(c.trading_symbols); })
      .catch((e) => toast.err(e.message));
  }, []);

  const toggleStrat = (id) => setPicked((prev) => prev.includes(id) ? prev.filter((s) => s !== id) : [...prev, id]);

  const run = async () => {
    if (picked.length === 0) { toast.err("Chọn ít nhất 1 strategy"); return; }
    setBusy(true);
    setResults(null);
    try {
      const res = await api("/backtest", {
        method: "POST",
        body: JSON.stringify({
          symbols, interval, limit: Number(limit), strategies: picked,
          start_balance: 10000, stake: Number(stake), sl_pct: Number(slPct),
          tp_pct: Number(tpPct), trailing, trail_pct: Number(trailPct),
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
    <div className="container section">
      <div className="page-header">
        <h1 className="page-title">Backtest</h1>
        <p className="page-subtitle">Chạy strategy trên nến lịch sử (không look-ahead) — đo win rate, PnL, drawdown, profit factor</p>
      </div>

      <div className="card">
        <div className="card-header">
          <h2 className="text-sm font-medium text-white">Parameters</h2>
        </div>
        <div className="card-body">
          <div className="grid-auto-sm lg:grid-cols-4 gap-4">
            <div className="lg:col-span-2">
              <label className="label">Symbols (phẩy cách nhau)</label>
              <input value={symbols} onChange={(e) => setSymbols(e.target.value.toUpperCase())} className={inputCls} />
            </div>
            <div>
              <label className="label">Khung thời gian</label>
              <select value={interval} onChange={(e) => setInterval_(e.target.value)} className={selectCls}>
                {INTERVALS.map((i) => <option key={i}>{i}</option>)}
              </select>
            </div>
            <div>
              <label className="label">Số nến</label>
              <select value={limit} onChange={(e) => setLimit(Number(e.target.value))} className={selectCls}>
                {[300, 500, 800, 1000].map((n) => <option key={n} value={n}>{n}</option>)}
              </select>
            </div>
            <div>
              <label className="label">Stake (USDT)</label>
              <input type="number" value={stake} onChange={(e) => setStake(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="label">Stop loss %</label>
              <input type="number" step="0.1" value={slPct} onChange={(e) => setSlPct(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="label">Take profit %</label>
              <input type="number" step="0.1" value={tpPct} onChange={(e) => setTpPct(e.target.value)} className={inputCls} />
            </div>
            <div className="flex items-end gap-2">
              <div className="flex-1">
                <label className="label">Trailing %</label>
                <input type="number" step="0.1" value={trailPct} disabled={!trailing} onChange={(e) => setTrailPct(e.target.value)} className={`${inputCls} ${!trailing ? "opacity-40" : ""}`} />
              </div>
              <button onClick={() => setTrailing(!trailing)} className={`btn ${trailing ? "btn-primary" : "btn-ghost"} h-10`}>
                {trailing ? "ON" : "OFF"}
              </button>
            </div>
            <div>
              <label className="label">Cooldown (nến)</label>
              <input type="number" min="0" value={cooldownBars} onChange={(e) => setCooldownBars(e.target.value)} className={inputCls} />
            </div>
          </div>

          <div className="mt-4">
            <label className="label">Strategies</label>
            <div className="flex flex-wrap gap-2">
              {STRATEGIES.map((s) => (
                <button key={s.id} onClick={() => toggleStrat(s.id)}
                  className={`px-3 py-1.5 rounded-lg border text-xs transition-colors ${
                    picked.includes(s.id) ? "border-amber-400/40 bg-amber-400/10 text-amber-300" : "border-[#1e2430] text-zinc-500 hover:text-white"
                  }`}
                >{s.label}</button>
              ))}
            </div>
          </div>

          <button onClick={run} disabled={busy} className="mt-5 btn-primary">
            <Play size={14} className={busy ? "animate-pulse" : ""} />
            {busy ? "Đang chạy…" : "Chạy backtest"}
          </button>
        </div>
      </div>

      {rows.length > 0 && (
        <div className="card">
          <div className="card-header flex items-center justify-between">
            <h2 className="text-sm font-medium text-white">Results</h2>
            <button onClick={() => {
              if (!rows.length) return;
              const head = ["symbol","strategy","interval","trades","win_rate","total_pnl","total_pnl_pct","max_drawdown","profit_factor","sharpe","expectancy","final_equity"];
              const esc = (v) => { const s = v == null ? "" : String(v); return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s; };
              const lines = [head.join(",")];
              for (const r of rows) {
                if (r.error) continue;
                const x = r.result;
                lines.push([r.symbol, r.strategy, r.interval, x.trades, x.win_rate.toFixed(2), x.total_pnl.toFixed(2), x.total_pnl_pct.toFixed(2), x.max_drawdown.toFixed(2), x.profit_factor >= 999 ? "inf" : x.profit_factor.toFixed(2), x.sharpe?.toFixed(2) ?? "", x.expectancy.toFixed(2), x.final_equity.toFixed(2)].map(esc).join(","));
              }
              const blob = new Blob(["\uFEFF" + lines.join("\n")], { type: "text/csv;charset=utf-8" });
              const url = URL.createObjectURL(blob);
              const a = document.createElement("a"); a.href = url; a.download = `backtest-${new Date().toISOString().slice(0,10)}.csv`; a.click();
              URL.revokeObjectURL(url);
              toast.ok(`Exported ${rows.filter(r=>!r.error).length} rows`);
            }} className="btn-secondary btn-sm">
              <Download size={13} /> CSV
            </button>
          </div>
          <div className="card-body p-0">
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th>Symbol</th><th>Strategy</th><th className="text-right">Trades</th><th className="text-right">Win rate</th><th className="text-right">PnL</th><th className="text-right">PnL %</th><th className="text-right">Max DD</th><th className="text-right">PF</th><th className="text-right">Sharpe</th><th className="text-right pr-4">Expectancy</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r, i) => {
                    if (r.error) {
                      return <tr key={i}><td className="py-3 px-4 text-white">{r.symbol}</td><td colSpan={9} className="text-rose-400">{r.error}</td></tr>;
                    }
                    const d = r.result;
                    const up = d.total_pnl >= 0;
                    return (
                      <tr key={i}>
                        <td className="py-3 px-4 font-medium text-white">{r.symbol}</td>
                        <td className="text-zinc-400">{r.strategy}</td>
                        <Metric label="trades" value={d.trades} />
                        <Metric label="win" value={`${d.win_rate.toFixed(1)}%`} tone={d.win_rate >= 50 ? "text-emerald-400" : "text-zinc-300"} />
                        <Metric label="pnl" value={`${up ? "+" : ""}${fmtUSD(d.total_pnl)}`} tone={up ? "text-emerald-400" : "text-rose-400"} />
                        <Metric label="pnl_pct" value={`${d.total_pnl_pct >= 0 ? "+" : ""}${d.total_pnl_pct.toFixed(2)}%`} tone={up ? "text-emerald-400" : "text-rose-400"} />
                        <Metric label="mdd" value={`${d.max_drawdown.toFixed(2)}%`} tone="text-amber-400/80" />
                        <Metric label="pf" value={d.profit_factor >= 999 ? "∞" : d.profit_factor.toFixed(2)} tone={d.profit_factor >= 1 ? "text-emerald-400" : "text-rose-400"} />
                        <Metric label="sharpe" value={d.sharpe ? d.sharpe.toFixed(2) : "—"} />
                        <Metric label="exp" value={`${d.expectancy >= 0 ? "+" : ""}${fmtUSD(d.expectancy)}`} tone={d.expectancy >= 0 ? "text-emerald-400" : "text-rose-400"} />
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {!results && !busy && (
        <div className="card">
          <div className="card-body">
            <div className="empty h-48">
              <div className="empty-icon"><FlaskConical size={48} className="text-zinc-600" /></div>
              <div className="empty-title">Sẵn sàng backtest</div>
              <div className="empty-desc">Cấu hình tham số ở trên rồi nhấn <b className="text-zinc-300">Chạy backtest</b>. Kết quả: trades, win rate, PnL, max drawdown, profit factor, Sharpe — từng symbol × strategy.</div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}