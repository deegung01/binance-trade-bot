"use client";

import { useEffect, useMemo, useState } from "react";
import { ArrowRightLeft, Coins, RefreshCw, Search } from "lucide-react";
import { api, fmtUSD } from "@/lib/api";
import { useToast } from "@/components/Toast";

const fmtQty = (v) =>
  v == null
    ? "—"
    : Number(v).toLocaleString("en-US", { maximumFractionDigits: v >= 100 ? 2 : 6 });

const SKIP_LABEL = {
  dust: "dust (quá nhỏ)",
  no_price: "không có giá",
  no_pair_or_filter: "không bán được (filter)",
};

export default function ConvertPage() {
  const toast = useToast();
  const [preview, setPreview] = useState(null);
  const [mode, setMode] = useState("all");
  const [minValue, setMinValue] = useState(1);
  const [search, setSearch] = useState("");
  const [picked, setPicked] = useState(new Set());
  const [busy, setBusy] = useState(false);
  const [execResults, setExecResults] = useState(null);

  const loadPreview = async () => {
    setBusy(true);
    try {
      const assets = [...picked];
      const res = await api("/convert/preview", {
        method: "POST",
        body: JSON.stringify({ assets, mode, min_value: Number(minValue) }),
      });
      setPreview(res);
      setExecResults(null);
      toast.ok(
        `Tìm thấy ${res.rows.filter((r) => r.tradable).length} coin bán được · ước tính ${fmtUSD(res.est_total_net)} USDT`
      );
    } catch (e) {
      toast.err(e.message);
    }
    setBusy(false);
  };

  const execute = async () => {
    if (!preview) return;
    const sellCount = preview.rows.filter((r) => r.tradable).length;
    if (sellCount === 0) { toast.err("Không có coin nào bán được"); return; }
    if (!confirm(
      `Bán ${sellCount} coin → USDT trên testnet?\n\nƯớc tính nhận: ${fmtUSD(preview.est_total_net)} USDT (sau phí)\n\nHành động này đặt lệnh SELL thật trên Binance Spot Testnet.`
    )) return;
    setBusy(true);
    try {
      const assets = [...picked];
      const res = await api("/convert/execute", {
        method: "POST",
        body: JSON.stringify({ assets, mode, min_value: Number(minValue) }),
      });
      setExecResults(res);
      const okCount = res.results.filter((x) => x.ok).length;
      const failCount = res.results.filter((x) => !x.ok).length;
      toast.ok(
        `Convert xong: ${okCount} coin → ${fmtUSD(res.total_net_usdt)} USDT` +
          (failCount ? ` · ${failCount} lỗi` : ""),
        8000
      );
    } catch (e) {
      toast.err(e.message);
    }
    setBusy(false);
  };

  const toggle = (asset) => {
    const next = new Set(picked);
    if (next.has(asset)) next.delete(asset);
    else next.add(asset);
    setPicked(next);
    setMode("selected");
  };

  const rows = useMemo(() => {
    if (!preview) return [];
    const q = search.trim().toUpperCase();
    if (!q) return preview.rows;
    return preview.rows.filter((r) => String(r.asset).toUpperCase().includes(q));
  }, [preview, search]);

  const tradableCount = preview ? preview.rows.filter((r) => r.tradable).length : 0;

  return (
    <div className="container section">
      <div className="page-header">
        <div>
          <h1 className="page-title">Convert → USDT</h1>
          <p className="page-subtitle">Bán hết coin (hoặc coin bạn chọn) về USDT · live testnet · tự bỏ qua dust</p>
        </div>
      </div>

      {/* controls */}
      <div className="card">
        <div className="card-body">
          <div className="grid-auto-sm lg:grid-cols-4 gap-4 items-end">
            <div>
              <label className="label">Phạm vi</label>
              <select value={mode} onChange={(e) => setMode(e.target.value)} className="select">
                <option value="all">Tất cả coin (trừ USDT)</option>
                <option value="selected">Chỉ coin tôi chọn bên dưới</option>
              </select>
            </div>
            <div>
              <label className="label">Bỏ qua coin dưới (USDT)</label>
              <input type="number" step="0.5" min="0" value={minValue} onChange={(e) => setMinValue(e.target.value)} className="input" />
            </div>
            <div className="lg:col-span-2 flex items-end gap-2">
              <button onClick={loadPreview} disabled={busy} className="btn-secondary flex-1">
                <RefreshCw size={14} className={busy ? "animate-spin" : ""} /> Xem trước (dry-run)
              </button>
              {preview && (
                <div className="text-right lg:w-48">
                  <div className="text-xs text-zinc-500">Ước tính nhận được (sau phí)</div>
                  <div className="text-xl font-semibold text-emerald-400 tabular">{fmtUSD(preview.est_total_net)}</div>
                  <div className="text-[11px] text-zinc-600">{tradableCount} coin bán được</div>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {!preview && !busy && (
        <div className="card">
          <div className="card-body">
            <div className="empty h-48">
              <div className="empty-icon"><Coins size={48} className="text-zinc-600" /></div>
              <div className="empty-title">Sẵn sàng convert</div>
              <div className="empty-desc">Nhấn <b className="text-zinc-300">Xem trước</b> để liệt kê coin trên tài khoản testnet và ước tính USDT nhận được. Chưa đặt lệnh nào cả.</div>
            </div>
          </div>
        </div>
      )}

      {preview && (
        <>
          <div className="card">
            <div className="card-header flex flex-wrap items-center justify-between gap-3">
              <h2 className="text-sm font-medium text-white">Danh sách coin</h2>
              <div className="relative w-full sm:w-64">
                <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
                <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Lọc coin…" className="input pl-8" />
              </div>
            </div>
            <div className="card-body p-0">
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      <th className="w-12">Chọn</th>
                      <th>Asset</th>
                      <th className="text-right">Qty (bán được)</th>
                      <th className="text-right">Giá (USDT)</th>
                      <th className="text-right">Giá trị</th>
                      <th className="text-right">Ước tính net</th>
                      <th className="text-right pr-4">Trạng thái</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((r) => (
                      <tr key={r.asset} className={`hover:bg-white/[0.02] ${!r.tradable ? "opacity-50" : ""}`}>
                        <td className="text-center">
                          <input type="checkbox" checked={picked.has(r.asset)} onChange={() => toggle(r.asset)} disabled={!r.tradable} className="accent-amber-400" />
                        </td>
                        <td className="font-medium text-white">{r.asset}</td>
                        <td className="text-right tabular text-zinc-300">{fmtQty(r.qty)}</td>
                        <td className="text-right tabular text-zinc-400">{r.price ? fmtUSD(r.price, r.price >= 100 ? 2 : 4) : "—"}</td>
                        <td className="text-right tabular text-zinc-300">{fmtUSD(r.usdt_value)}</td>
                        <td className="text-right tabular text-emerald-400/90">{r.tradable ? fmtUSD(r.est_net) : "—"}</td>
                        <td className="text-right pr-4">
                          {r.tradable ? (
                            <span className="badge-ok">sell</span>
                          ) : (
                            <span className="badge-neutral" title={r.skip_reason}>{SKIP_LABEL[r.skip_reason] || "bỏ qua"}</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {rows.length === 0 && <div className="empty h-24"><div className="empty-title">Không có coin nào khớp bộ lọc</div></div>}
              </div>
            </div>
          </div>

          <button onClick={execute} disabled={busy || tradableCount === 0} className={`btn w-full ${tradableCount > 0 ? "btn-primary" : "btn-ghost"}`}>
            <ArrowRightLeft size={14} />
            {mode === "all" ? `Convert tất cả → USDT (${tradableCount} coin)` : `Convert ${[...picked].length} coin đã chọn → USDT`}
          </button>
        </>
      )}

      {/* results */}
      {execResults && (
        <div className="card">
          <div className="card-header flex items-center justify-between">
            <h2 className="text-sm font-medium text-white">Kết quả convert</h2>
            <span className="text-sm font-semibold text-emerald-400 tabular">+{fmtUSD(execResults.total_net_usdt)} USDT</span>
          </div>
          <div className="card-body p-0">
            <div className="table-wrap">
              <table className="table">
                <thead><tr><th>Asset</th><th className="text-right">Qty</th><th className="text-right">Net (USDT)</th><th className="text-right pr-2">Kết quả</th></tr></thead>
                <tbody>
                  {execResults.results.map((r, i) => (
                    <tr key={i}>
                      <td className="font-medium text-white">{r.asset}</td>
                      <td className="text-right tabular text-zinc-300">{fmtQty(r.executed_qty ?? r.qty)}</td>
                      <td className="text-right tabular text-zinc-300">{r.ok ? fmtUSD(r.net_usdt) : "—"}</td>
                      <td className="text-right pr-2">{r.ok ? <span className="badge-ok">OK</span> : <span className="badge-err" title={r.error}>lỗi</span>}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}