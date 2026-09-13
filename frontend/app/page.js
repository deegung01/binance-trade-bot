"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  ArrowDownRight,
  ArrowUpRight,
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

function StatCard({ icon: Icon, label, value, sub, tone = "default" }) {
  return (
    <div className="panel p-4">
      <div className="flex items-center justify-between text-xs text-zinc-500">
        <span className="uppercase tracking-wide">{label}</span>
        {Icon && <Icon size={14} className="text-zinc-600" />}
      </div>
      <div
        className={`mt-2 text-xl font-semibold tabular ${
          tone === "up" ? "text-emerald-400" : tone === "down" ? "text-rose-400" : "text-white"
        }`}
      >
        {value}
      </div>
      {sub && <div className="mt-1 text-xs text-zinc-500">{sub}</div>}
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
      <div className="panel p-8 text-center">
        <p className="text-rose-400">Backend not reachable: {error}</p>
        <p className="mt-2 text-sm text-zinc-500">
          Make sure the backend is running, then refresh.
        </p>
      </div>
    );
  }
  if (!status) return <div className="text-zinc-500 text-sm">Loading…</div>;

  const st = status.stats;
  const up = status.profit >= 0;

  return (
    <div className="space-y-6 max-w-[1400px]">
      {/* header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-white">Overview</h1>
          <p className="text-sm text-zinc-500">
            Paper trading on Binance Spot Testnet · last cycle {fmtTime(status.last_cycle?.ts)} UTC
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={load}
            className="panel px-3 py-2 text-sm text-zinc-300 hover:text-white flex items-center gap-2"
          >
            <RefreshCw size={14} /> Refresh
          </button>
          <button
            onClick={toggleRunning}
            disabled={busy}
            className={`px-4 py-2 rounded-lg text-sm font-medium flex items-center gap-2 ${
              status.running
                ? "bg-rose-500/15 text-rose-300 hover:bg-rose-500/25"
                : "bg-emerald-500/15 text-emerald-300 hover:bg-emerald-500/25"
            }`}
          >
            {status.running ? <Pause size={14} /> : <Play size={14} />}
            {status.running ? "Pause bot" : "Start bot"}
          </button>
        </div>
      </div>

      {/* stat cards */}
      <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-4">
        <StatCard
          icon={Wallet}
          label="Equity"
          value={fmtUSD(status.equity)}
          sub={`start ${fmtUSD(status.start_balance, 0)}`}
        />
        <StatCard
          icon={up ? TrendingUp : TrendingDown}
          label="Profit"
          value={
            <span className={pnlColor(status.profit)}>
              {up ? "+" : ""}
              {fmtUSD(status.profit)} ({fmtPct(status.profit_pct)})
            </span>
          }
          tone={up ? "up" : "down"}
        />
        <StatCard icon={Coins} label="Cash" value={fmtUSD(status.cash)} sub={`in positions ${fmtUSD(status.positions_value)}`} />
        <StatCard icon={Percent} label="Win rate" value={`${st.win_rate}%`} sub={`${st.wins}W / ${st.losses}L`} />
        <StatCard icon={Target} label="Total PnL (closed)" value={fmtUSD(st.total_pnl)} sub={`${st.closed_trades} closed trades`} />
        <StatCard icon={TrendingDown} label="Max drawdown" value={`${st.max_drawdown}%`} sub={`best ${fmtUSD(st.best_trade)} / worst ${fmtUSD(st.worst_trade)}`} />
      </div>

      <div className="grid lg:grid-cols-3 gap-4">
        {/* equity chart */}
        <div className="panel p-4 lg:col-span-2">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-medium text-white">Equity curve</h2>
            <span className="text-xs text-zinc-500">{equity.length} snapshots</span>
          </div>
          <EquityChart points={equity} />
        </div>

        {/* open positions */}
        <div className="panel p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-medium text-white">Open positions</h2>
            <Link href="/trades" className="text-xs text-amber-400 hover:underline">
              view all →
            </Link>
          </div>
          {status.open_trades.length === 0 ? (
            <p className="text-sm text-zinc-500 py-6 text-center">No open positions</p>
          ) : (
            <div className="space-y-2">
              {status.open_trades.map((t) => {
                const price = status.last_cycle?.prices?.[t.symbol] || t.entry_price;
                const cur = ((price - t.entry_price) / t.entry_price) * 100;
                return (
                  <div key={t.id} className="rounded-lg bg-white/[0.03] border border-[#1e2430] p-3">
                    <div className="flex justify-between items-center">
                      <span className="font-medium text-white text-sm">{t.symbol}</span>
                      <span className={`text-xs font-medium tabular ${pnlColor(cur)}`}>
                        {fmtPct(cur)} · {fmtUSD(((price - t.entry_price) * t.qty))}
                      </span>
                    </div>
                    <div className="mt-2 text-xs text-zinc-500 grid grid-cols-2 gap-y-1">
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

      {/* market prices */}
      <div className="panel p-4">
        <h2 className="text-sm font-medium text-white mb-3">Watchlist (testnet prices)</h2>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          {Object.entries(status.last_cycle?.prices || {}).map(([sym, price]) => (
            <div key={sym} className="rounded-lg bg-white/[0.03] border border-[#1e2430] p-3">
              <div className="text-xs text-zinc-500">{sym}</div>
              <div className="text-white font-medium tabular mt-0.5">{fmtUSD(price)}</div>
            </div>
          ))}
        </div>
      </div>

      {/* strategy info footer */}
      <div className="panel p-4 flex flex-wrap items-center gap-x-8 gap-y-2 text-xs text-zinc-500">
        <span className="flex items-center gap-1.5"><Bot size={13} /> strategy: <b className="text-zinc-300">{status.last_cycle?.strategy || "—"}</b></span>
        <span className="flex items-center gap-1.5"><Clock size={13} /> timeframe: <b className="text-zinc-300">{status.last_cycle?.timeframe || "—"}</b></span>
        <span className="flex items-center gap-1.5"><Wallet size={13} /> open trades: <b className="text-zinc-300">{status.open_trades.length}</b></span>
        <span className="flex items-center gap-1.5">
          mode: <b className={status.mode === "live" ? "text-rose-300" : "text-sky-300"}>{status.mode}</b>
        </span>
      </div>
    </div>
  );
}
