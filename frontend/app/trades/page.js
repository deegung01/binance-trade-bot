"use client";

import { useEffect, useMemo, useState } from "react";
import { RefreshCw, Search } from "lucide-react";
import { api, fmtUSD, fmtPct, fmtNum, fmtDate, pnlColor } from "@/lib/api";
import { useToast } from "@/components/Toast";

export default function TradesPage() {
  const toast = useToast();
  const [trades, setTrades] = useState([]);
  const [status, setStatus] = useState(null);
  const [filter, setFilter] = useState("all");
  const [search, setSearch] = useState("");
  const [strategyFilter, setStrategyFilter] = useState("");
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const [t, s] = await Promise.all([api("/trades?limit=300"), api("/status")]);
      setTrades(t.trades);
      setStatus(s);
    } catch (e) {
      toast.err(e.message);
    }
  };

  useEffect(() => {
    load();
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const closeTrade = async (id) => {
    if (!confirm(`Đóng toàn bộ lệnh #${id}?`)) return;
    setBusy(true);
    try {
      await api(`/sell/${id}`, { method: "POST" });
      toast.ok(`Đã đóng lệnh #${id}`);
      await load();
    } catch (e) {
      toast.err(e.message);
    }
    setBusy(false);
  };

  const partialClose = async (id, pct) => {
    if (!confirm(`Đóng ${pct}% lệnh #${id}? Phần còn lại giữ nguyên SL/TP.`)) return;
    setBusy(true);
    try {
      const res = await api(`/sell/${id}/partial`, {
        method: "POST",
        body: JSON.stringify({ pct }),
      });
      toast.ok(`Đóng ${pct}% #${id}: +${fmtUSD(res.net_usdt)} USDT`);
      await load();
    } catch (e) {
      toast.err(e.message);
    }
    setBusy(false);
  };

  const strategies = useMemo(() => {
    const s = new Set(trades.map((t) => t.strategy).filter(Boolean));
    return [...s].sort();
  }, [trades]);

  const shown = useMemo(() => {
    const q = search.trim().toUpperCase();
    return trades.filter((t) => {
      if (filter !== "all" && t.status !== filter) return false;
      if (strategyFilter && t.strategy !== strategyFilter) return false;
      if (q && !t.symbol.includes(q)) return false;
      return true;
    });
  }, [trades, filter, strategyFilter, search]);

  const prices = status?.last_cycle?.prices || {};

  return (
    <div className="space-y-4 max-w-[1400px]">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-white">Trades</h1>
          <p className="text-sm text-zinc-500">Tất cả lệnh của bot + manual</p>
        </div>
        <div className="flex gap-2 items-center flex-wrap">
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
          <select
            value={strategyFilter}
            onChange={(e) => setStrategyFilter(e.target.value)}
            className="bg-[#11151d] border border-[#1e2430] rounded-lg px-2.5 py-1.5 text-xs text-white outline-none focus:border-amber-400/50"
          >
            <option value="">mọi strategy</option>
            {strategies.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </select>
          <div className="relative">
            <Search size={13} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-500" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Symbol…"
              className="w-32 bg-[#11151d] border border-[#1e2430] rounded-lg pl-7 pr-2 py-1.5 text-xs text-white outline-none focus:border-amber-400/50"
            />
          </div>
          <button onClick={load} className="panel px-3 py-1.5 text-sm text-zinc-300 hover:text-white flex items-center gap-2">
            <RefreshCw size={13} /> Refresh
          </button>
        </div>
      </div>

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
                  <td className="text-right text-zinc-500 text-[11px] max-w-[180px] truncate">
                    {t.signal_reason || t.exit_reason || "—"}
                    {t.meta?.grid_count > 0 && (
                      <span className="ml-1 text-amber-400" title={`DCA ${t.meta.grid_count} lần`}>
                        [{t.meta.grid_count}x]
                      </span>
                    )}
                  </td>
                  <td className="text-right pr-4">
                    {t.status === "open" && (
                      <div className="flex gap-1 justify-end">
                        <button
                          onClick={() => partialClose(t.id, 50)}
                          disabled={busy}
                          title="Đóng 50% lệnh, phần còn lại giữ SL/TP"
                          className="text-[11px] px-2 py-1 rounded bg-amber-500/15 text-amber-300 hover:bg-amber-500/25"
                        >
                          50%
                        </button>
                        <button
                          onClick={() => closeTrade(t.id)}
                          disabled={busy}
                          className="text-[11px] px-2 py-1 rounded bg-rose-500/15 text-rose-300 hover:bg-rose-500/25"
                        >
                          Close
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {shown.length === 0 && (
          <div className="py-10 text-center text-sm text-zinc-500">Không có lệnh nào khớp bộ lọc</div>
        )}
      </div>
    </div>
  );
}
