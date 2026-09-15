"use client";

import { useEffect, useMemo, useState } from "react";
import { Download, RefreshCw, Search, ChevronDown } from "lucide-react";
import { api, fmtUSD, fmtPct, fmtNum, fmtDate, pnlColor } from "@/lib/api";
import { useToast } from "@/components/Toast";

const inputCls = "input";
const selectCls = "select";

export default function TradesPage() {
  const toast = useToast();
  const [trades, setTrades] = useState([]);
  const [status, setStatus] = useState(null);
  const [filter, setFilter] = useState("all");
  const [search, setSearch] = useState("");
  const [strategyFilter, setStrategyFilter] = useState("");
  const [busy, setBusy] = useState(false);
  const [sortConfig, setSortConfig] = useState({ key: "opened_at", dir: "desc" });

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

  const exportCSV = () => {
    if (shown.length === 0) { toast.err("Không có lệnh nào để export"); return; }
    const head = ["id", "symbol", "status", "mode", "strategy", "qty", "entry_price", "exit_price", "stop_loss", "take_profit", "stake", "pnl", "pnl_pct", "exit_reason", "opened_at", "closed_at"];
    const esc = (v) => { const s = v == null ? "" : String(v); return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s; };
    const lines = [head.join(",")];
    for (const t of shown) {
      lines.push([t.id, t.symbol, t.status, t.mode, t.strategy, t.qty, t.entry_price, t.exit_price ?? "", t.stop_loss, t.take_profit, t.stake, t.pnl, t.pnl_pct, t.exit_reason ?? "", t.opened_at, t.closed_at ?? ""].map(esc).join(","));
    }
    const blob = new Blob(["\uFEFF" + lines.join("\n")], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `trades-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
    toast.ok(`Exported ${shown.length} lệnh ra CSV`);
  };

  const handleSort = (key) => {
    setSortConfig((prev) => ({
      key,
      dir: prev.key === key && prev.dir === "asc" ? "desc" : "asc",
    }));
  };

  const strategies = useMemo(() => [...new Set(trades.map((t) => t.strategy).filter(Boolean))].sort(), [trades]);

  const shown = useMemo(() => {
    const q = search.trim().toUpperCase();
    let arr = trades.filter((t) => {
      if (filter !== "all" && t.status !== filter) return false;
      if (strategyFilter && t.strategy !== strategyFilter) return false;
      if (q && !t.symbol.includes(q)) return false;
      return true;
    });
    // sort
    arr.sort((a, b) => {
      const av = a[sortConfig.key], bv = b[sortConfig.key];
      if (av == null && bv == null) return 0;
      if (av == null) return 1;
      if (bv == null) return -1;
      const cmp = av < bv ? -1 : av > bv ? 1 : 0;
      return sortConfig.dir === "asc" ? cmp : -cmp;
    });
    return arr;
  }, [trades, filter, strategyFilter, search, sortConfig]);

  const prices = status?.last_cycle?.prices || {};

  return (
    <div className="container section">
      <div className="page-header">
        <div>
          <h1 className="page-title">Trades</h1>
          <p className="page-subtitle">Tất cả lệnh của bot + manual orders</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex rounded-lg overflow-hidden border border-[#1e2430]" role="group" aria-label="Status filter">
            {["all", "open", "closed"].map((f) => (
              <button key={f} onClick={() => setFilter(f)} className={`px-3 py-1.5 text-xs ${filter === f ? "bg-amber-400/15 text-amber-300" : "text-zinc-400 hover:text-white"}`}>{f}</button>
            ))}
          </div>
          <select value={strategyFilter} onChange={(e) => setStrategyFilter(e.target.value)} className={selectCls} style={{minWidth: 160}}>
            <option value="">mọi strategy</option>
            {strategies.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
          <div className="relative">
            <Search size={13} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-500" />
            <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Symbol…" className="input pl-7 w-40" />
          </div>
          <button onClick={exportCSV} className="btn-secondary btn-sm" title="Export CSV"><Download size={13} /> CSV</button>
          <button onClick={load} className="btn-ghost btn-sm" aria-label="Refresh"><RefreshCw size={13} /></button>
        </div>
      </div>

      <div className="card">
        <div className="card-body p-0">
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th onClick={() => handleSort("id")} className="cursor-pointer select-none">ID <span className={`inline ml-1 text-xs ${sortConfig.key==="id" ? (sortConfig.dir==="asc"?"↑":"↓") : ""}`} /></th>
                  <th onClick={() => handleSort("symbol")} className="cursor-pointer select-none">Symbol <span className={`inline ml-1 text-xs ${sortConfig.key==="symbol" ? (sortConfig.dir==="asc"?"↑":"↓") : ""}`} /></th>
                  <th onClick={() => handleSort("opened_at")} className="cursor-pointer select-none">Opened <span className={`inline ml-1 text-xs ${sortConfig.key==="opened_at" ? (sortConfig.dir==="asc"?"↑":"↓") : ""}`} /></th>
                  <th className="text-right">Qty</th>
                  <th className="text-right">Entry</th>
                  <th className="text-right">Current/Exit</th>
                  <th className="text-right">SL / TP</th>
                  <th onClick={() => handleSort("pnl")} className="text-right cursor-pointer select-none">PnL <span className={`inline ml-1 text-xs ${sortConfig.key==="pnl" ? (sortConfig.dir==="asc"?"↑":"↓") : ""}`} /></th>
                  <th className="text-right max-w-[180px]">Reason</th>
                  <th className="text-right pr-4">Action</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((t) => {
                  const cur = t.status === "open" ? prices[t.symbol] || t.entry_price : t.exit_price;
                  const unreal = t.status === "open" ? (cur - t.entry_price) * t.qty : t.pnl;
                  const upct = t.status === "open" ? ((cur - t.entry_price) / t.entry_price) * 100 : t.pnl_pct;
                  return (
                    <tr key={t.id}>
                      <td className="text-zinc-500 font-mono">#{t.id}</td>
                      <td className="text-white font-medium">{t.symbol}</td>
                      <td className="text-zinc-400">{fmtDate(t.opened_at)}</td>
                      <td className="text-right tabular text-zinc-300">{fmtNum(t.qty, 6)}</td>
                      <td className="text-right tabular text-zinc-300">{fmtUSD(t.entry_price)}</td>
                      <td className="text-right tabular text-zinc-300">{fmtUSD(cur)}</td>
                      <td className="text-right tabular text-[11px]"><span className="text-rose-400/80">{fmtUSD(t.stop_loss)}</span> / <span className="text-emerald-400/80">{fmtUSD(t.take_profit)}</span></td>
                      <td className={`text-right tabular font-medium ${pnlColor(unreal)}`}>{unreal > 0 ? "+" : ""}{fmtUSD(unreal)} <span className="text-[10px] opacity-70">{fmtPct(upct)}</span></td>
                      <td className="text-right text-zinc-500 text-[11px] max-w-[180px] truncate pr-2">{t.signal_reason || t.exit_reason || "—"}{t.meta?.grid_count > 0 && <span className="ml-1 text-amber-400" title={`DCA ${t.meta.grid_count} lần`}> [{t.meta.grid_count}x]</span>}</td>
                      <td className="text-right pr-4">
                        {t.status === "open" && (
                          <div className="flex items-center justify-end gap-1">
                            <button onClick={() => partialClose(t.id, 50)} disabled={busy} title="Đóng 50% lệnh, phần còn lại giữ SL/TP" className="btn-secondary btn-sm">50%</button>
                            <button onClick={() => closeTrade(t.id)} disabled={busy} className="btn-danger btn-sm">Close</button>
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            {shown.length === 0 && <div className="empty h-32"><div className="empty-title">Không có lệnh nào khớp bộ lọc</div></div>}
          </div>
        </div>
      </div>
    </div>
  );
}