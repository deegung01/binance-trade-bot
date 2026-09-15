"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  ArrowDownRight,
  ArrowUpRight,
  AlertTriangle,
  Bot,
  Clock,
  Coins,
  Percent,
  Play,
  Pause,
  RefreshCw,
  Target,
  TrendingDown,
  TrendingUp,
  Wallet,
} from "lucide-react";
import { api, fmtUSD, fmtPct, fmtNum, fmtDate, fmtTime, pnlColor } from "@/lib/api";
import EquityChart from "@/components/EquityChart";

const fmtQty = (v) =>
  v == null
    ? "—"
    : Number(v).toLocaleString("en-US", { maximumFractionDigits: v >= 100 ? 2 : 6 });

function StatCard({ icon: Icon, label, value, sub, tone = "default" }) {
  return (
    <div className="stat-card">
      <div className="flex items-center justify-between">
        <span className="stat-label">{label}</span>
        {Icon && <Icon size={14} className="text-zinc-600" />}
      </div>
      <div className={`stat-value ${tone === "up" ? "text-emerald-400" : tone === "down" ? "text-rose-400" : ""}`}>
        {value}
      </div>
      {sub && <div className="stat-sub">{sub}</div>}
    </div>
  );
}

export default function Overview() {
  const [status, setStatus] = useState(null);
  const [equity, setEquity] = useState([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const [s, e] = await Promise.all([api("/status"), api("/equity?limit=300")]);
      setStatus(s);
      setEquity(e.points);
      setError("");
    } catch (err) {
      setError(err.message);
    }
  };

  useEffect(() => {
    load();
    const t = setInterval(load, 5000);
    return () => clearInterval(t);
  }, []);

  const toggleRunning = async () => {
    if (!status) return;
    setBusy(true);
    try {
      await api("/config", {
        method: "POST",
        body: JSON.stringify({ bot_running: !status.running }),
      });
      await load();
    } catch (e) {
      setError(e.message);
    }
    setBusy(false);
  };

  if (error && !status) {
    return (
      <div className="empty">
        <div className="empty-icon"><AlertTriangle size={48} className="text-rose-500/50" /></div>
        <div className="empty-title">Backend not reachable</div>
        <div className="empty-desc">{error}</div>
        <button onClick={load} className="btn-primary mt-4">Retry</button>
      </div>
    );
  }
  if (!status) return <div className="empty"><div className="empty-icon"><div className="skeleton w-12 h-12 rounded-full" /></div><div className="empty-title">Loading…</div></div>;

  const st = status.stats;
  const up = status.profit >= 0;

  return (
    <div className="container section">
      <div className="page-header">
        <div>
          <h1 className="page-title">Overview</h1>
          <p className="page-subtitle">
            Paper trading on Binance Spot Testnet · last cycle {fmtTime(status.last_cycle?.ts)} UTC
          </p>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <button onClick={load} className="btn-secondary btn-sm" aria-label="Refresh">
            <RefreshCw size={14} /> Refresh
          </button>
          <button
            onClick={toggleRunning}
            disabled={busy}
            className={`btn ${status.running ? "btn-danger" : "btn-primary"}`}
          >
            {status.running ? <Pause size={14} /> : <Play size={14} />}
            {status.running ? "Pause bot" : "Start bot"}
          </button>
        </div>
      </div>

      {/* stat cards */}
      <div className="grid-auto-sm lg:grid-cols-6">
        <StatCard icon={Wallet} label="Equity" value={fmtUSD(status.equity)} sub={`start ${fmtUSD(status.start_balance, 0)}`} />
        <StatCard icon={up ? TrendingUp : TrendingDown} label="Profit" value={<span className={pnlColor(status.profit)}>{up ? "+" : ""}{fmtUSD(status.profit)} ({fmtPct(status.profit_pct)})</span>} tone={up ? "up" : "down"} />
        <StatCard icon={Coins} label={status.mode === "live" ? "USDT (testnet)" : "Cash"} value={fmtUSD(status.cash)} sub={status.mode === "live" ? `in coins ${fmtUSD(status.positions_value)}` : `in positions ${fmtUSD(status.positions_value)}`} />
        <StatCard icon={Percent} label="Win rate" value={`${st.win_rate}%`} sub={`${st.wins}W / ${st.losses}L`} />
        <StatCard icon={Target} label="Total PnL (closed)" value={fmtUSD(st.total_pnl)} sub={`${st.closed_trades} closed trades`} />
        <StatCard icon={TrendingDown} label="Max drawdown" value={`${st.max_drawdown}%`} sub={`best ${fmtUSD(st.best_trade)} / worst ${fmtUSD(st.worst_trade)}`} />
      </div>

      <div className="grid-auto lg:grid-cols-[2fr_1fr] gap-4">
        {/* equity chart */}
        <div className="card">
          <div className="card-header flex items-center justify-between">
            <h2 className="text-sm font-medium text-white">Equity curve</h2>
            <span className="text-xs text-zinc-500">{equity.length} snapshots</span>
          </div>
          <div className="card-body h-64">
            <EquityChart points={equity} />
          </div>
        </div>

        {/* open positions */}
        <div className="card">
          <div className="card-header flex items-center justify-between">
            <h2 className="text-sm font-medium text-white">Open positions</h2>
            <Link href="/trades" className="text-xs text-amber-400 hover:underline">view all →</Link>
          </div>
          <div className="card-body p-0">
            {status.open_trades.length === 0 ? (
              <div className="empty h-48">
                <div className="empty-icon"><Wallet size={32} /></div>
                <div className="empty-title">No open positions</div>
                <div className="empty-desc">Start the bot or place a manual order</div>
              </div>
            ) : (
              <div className="divide-y divide-[#161c28]">
                {status.open_trades.map((t) => {
                  const price = status.last_cycle?.prices?.[t.symbol] || t.entry_price;
                  const cur = ((price - t.entry_price) / t.entry_price) * 100;
                  return (
                    <div key={t.id} className="p-4 hover:bg-white/[0.02]">
                      <div className="flex items-center justify-between">
                        <span className="font-medium text-white text-sm">{t.symbol}</span>
                        <span className={`text-xs font-medium tabular ${pnlColor(cur)}`}>
                          {fmtPct(cur)} · {fmtUSD(((price - t.entry_price) * t.qty))}
                        </span>
                      </div>
                      <div className="mt-2 grid grid-cols-4 gap-y-1 text-xs text-zinc-500">
                        <span>Qty {fmtNum(t.qty, 6)}</span>
                        <span>Entry {fmtUSD(t.entry_price)}</span>
                        <span className="text-rose-400/80">SL {fmtUSD(t.stop_loss)}</span>
                        <span className="text-emerald-400/80">TP {fmtUSD(t.take_profit)}</span>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* balances (live mode) */}
      {status.mode === "live" && status.balances?.length > 0 && (
        <div className="card">
          <div className="card-header flex items-center justify-between">
            <h2 className="text-sm font-medium text-white">Testnet account balances</h2>
            <span className="text-xs text-zinc-500">{status.balances.length} coins · total {fmtUSD(status.equity)}</span>
          </div>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Asset</th>
                  <th className="text-right">Free</th>
                  <th className="text-right">Locked</th>
                  <th className="text-right">Price (USDT)</th>
                  <th className="text-right">Value (USDT)</th>
                  <th className="text-right pr-4">%</th>
                </tr>
              </thead>
              <tbody>
                {status.balances.map((b) => {
                  const pct = status.equity > 0 ? (b.usdt_value / status.equity) * 100 : 0;
                  return (
                    <tr key={b.asset}>
                      <td className="font-medium text-white">{b.asset}</td>
                      <td className="text-right tabular text-zinc-300">{fmtQty(b.free)}</td>
                      <td className="text-right tabular text-zinc-500">{b.locked > 0 ? fmtQty(b.locked) : "—"}</td>
                      <td className="text-right tabular text-zinc-400">{b.price ? fmtUSD(b.price, b.price >= 100 ? 2 : 4) : "—"}</td>
                      <td className="text-right tabular text-zinc-300">{fmtUSD(b.usdt_value)}</td>
                      <td className="text-right tabular text-zinc-500 pr-4">{pct.toFixed(1)}%</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* market prices */}
      <div className="card">
        <h2 className="card-header text-sm font-medium text-white">Watchlist (testnet prices)</h2>
        <div className="card-body">
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {Object.entries(status.last_cycle?.prices || {}).map(([sym, price]) => (
              <div key={sym} className="rounded-lg border border-[#1e2430] p-3 hover:bg-white/[0.02] transition-colors">
                <div className="text-xs text-zinc-500">{sym}</div>
                <div className="text-white font-medium tabular mt-0.5">{fmtUSD(price)}</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* strategy info footer */}
      <div className="card p-4">
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-xs text-zinc-500">
          <span className="flex items-center gap-1.5"><Bot size={13} /> strategy: <b className="text-zinc-300">{status.last_cycle?.strategy || "—"}</b></span>
          <span className="flex items-center gap-1.5"><Clock size={13} /> timeframe: <b className="text-zinc-300">{status.last_cycle?.timeframe || "—"}</b></span>
          <span className="flex items-center gap-1.5"><Wallet size={13} /> open trades: <b className="text-zinc-300">{status.open_trades.length}</b></span>
          <span className="flex items-center gap-1.5">
            mode: <b className={status.mode === "live" ? "text-rose-300" : "text-sky-300"}>{status.mode}</b>
          </span>
        </div>
      </div>
    </div>
  );
}