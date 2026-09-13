"use client";

import { useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";
import { api, fmtUSD, fmtPct, fmtNum, fmtDate, pnlColor } from "@/lib/api";

export default function TradesPage() {
  const [trades, setTrades] = useState([]);
  const [status, setStatus] = useState(null);
  const [filter, setFilter] = useState("all");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const load = async () => {
    try {
      const [t, s] = await Promise.all([api("/trades?limit=300"), api("/status")]);
      setTrades(t.trades);
      setStatus(s);
    } catch (e) {
      setMsg(e.message);
    }
  };

  useEffect(() => {
    load();
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
  }, []);

  const closeTrade = async (id) => {
    setBusy(true);
    try {
      await api(`/sell/${id}`, { method: "POST" });
      await load();
    } catch (e) {
      setMsg(e.message);
    }
    setBusy(false);
  };

  const shown = trades.filter((t) => filter === "all" || t.status === filter);
  const prices = status?.last_cycle?.prices || {};

  return (
    <div className="space-y-4 max-w-[1400px]">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-white">Trades</h1>
          <p className="text-sm text-zinc-500">All bot trades + manual orders</p>
        </div>
        <div className="flex gap-2 items-center">
          <div className="flex rounded-lg overflow-hidden border border-[#1e2430] text-xs">
            {["all", "open", "closed"].map((f) => (
              <button
                key={f}
                onClick={() => setFilter(f)}
                className={`px-3 py-1.5 ${
                  filter === f ? "bg-amber-400/15 text-amber-300" : "text-zinc-400 hover:text-white"
                }`}
              >
                {f}
              </button>
            ))}
          </div>
          <button onClick={load} className="panel px-3 py-1.5 text-sm text-zinc-300 hover:text-white flex items-center gap-2">
            <RefreshCw size={13} /> Refresh
          </button>
        </div>
      </div>

      {msg && <div className="text-xs text-rose-400">{msg}</div>}

      <div className="panel overflow-x-auto">
        <table className="w-full text-xs">
          <thead>
            <tr className="text-zinc-500 border-b border-[#1e2430]">
              <th className="text-left py-3 px-4 font-normal">ID</th>
              <th className="text-left font-normal">Symbol</th>
              <th className="text-left font-normal">Opened</th>
              <th className="text-right font-normal">Qty</th>
              <th className="text-right font-normal">Entry</th>
              <th className="text-right font-normal">Current/Exit</th>
              <th className="text-right font-normal">SL / TP</th>
              <th className="text-right font-normal">PnL</th>
              <th className="text-right font-normal">Reason</th>
              <th className="text-right font-normal pr-4">Action</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((t) => {
              const cur = t.status === "open" ? prices[t.symbol] || t.entry_price : t.exit_price;
              const unreal = t.status === "open" ? (cur - t.entry_price) * t.qty : t.pnl;
              const upct = t.status === "open" ? ((cur - t.entry_price) / t.entry_price) * 100 : t.pnl_pct;
              return (
                <tr key={t.id} className="border-b border-[#161c28] hover:bg-white/[0.02]">
                  <td className="py-2.5 px-4 text-zinc-500">#{t.id}</td>
                  <td className="text-white font-medium">{t.symbol}</td>
                  <td className="text-zinc-400">{fmtDate(t.opened_at)}</td>
                  <td className="text-right tabular text-zinc-300">{fmtNum(t.qty, 6)}</td>
                  <td className="text-right tabular text-zinc-300">{fmtUSD(t.entry_price)}</td>
                  <td className="text-right tabular text-zinc-300">{fmtUSD(cur)}</td>
                  <td className="text-right tabular text-[11px]">
                    <span className="text-rose-400/80">{fmtUSD(t.stop_loss)}</span>
                    <span className="text-zinc-600"> / </span>
                    <span className="text-emerald-400/80">{fmtUSD(t.take_profit)}</span>
                  </td>
                  <td className={`text-right tabular font-medium ${pnlColor(unreal)}`}>
                    {unreal > 0 ? "+" : ""}
                    {fmtUSD(unreal)} <span className="text-[10px] opacity-70">{fmtPct(upct)}</span>
                  </td>
                  <td className="text-right text-zinc-500 text-[11px] max-w-[180px] truncate">{t.signal_reason || t.exit_reason || "—"}</td>
                  <td className="text-right pr-4">
                    {t.status === "open" && (
                      <button
                        onClick={() => closeTrade(t.id)}
                        disabled={busy}
                        className="text-[11px] px-2 py-1 rounded bg-rose-500/15 text-rose-300 hover:bg-rose-500/25"
                      >
                        Close
                      </button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {shown.length === 0 && (
          <div className="py-10 text-center text-sm text-zinc-500">No trades yet</div>
        )}
      </div>
    </div>
  );
}
