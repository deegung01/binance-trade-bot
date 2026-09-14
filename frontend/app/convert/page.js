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
  const [mode, setMode] = useState("all"); // all | selected
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
    if (sellCount === 0) {
      toast.err("Không có coin nào bán được");
      return;
    }
    if (
      !confirm(
        `Bán ${sellCount} coin → USDT trên testnet?\n\nƯớc tính nhận: ${fmtUSD(
          preview.est_total_net
        )} USDT (sau phí)\n\nHành động này đặt lệnh SELL thật trên Binance Spot Testnet.`
      )
    )
      return;
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
    <div className="space-y-4 max-w-[1400px]">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-white">Convert → USDT</h1>
          <p className="text-sm text-zinc-500">
            Bán hết coin (hoặc coin bạn chọn) về USDT · live testnet · tự bỏ qua dust
          </p>
        </div>
      </div>

      {/* controls */}
      <div className="panel p-4 flex flex-wrap items-end gap-4">
        <div>
          <label className="text-xs text-zinc-500 block">Phạm vi</label>
          <select
            value={mode}
            onChange={(e) => setMode(e.target.value)}
            className="mt-1 bg-[#0d1117] border border-[#1e2430] rounded-lg px-3 py-2 text-sm text-white outline-none focus:border-amber-400/50"
          >
            <option value="all">Tất cả coin (trừ USDT)</option>
            <option value="selected">Chỉ coin tôi chọn bên dưới</option>
          </select>
        </div>
        <div>
          <label className="text-xs text-zinc-500 block">Bỏ qua coin dưới (USDT)</label>
          <input
            type="number"
            step="0.5"
            min="0"
            value={minValue}
            onChange={(e) => setMinValue(e.target.value)}
            className="mt-1 w-28 bg-[#0d1117] border border-[#1e2430] rounded-lg px-3 py-2 text-sm text-white outline-none focus:border-amber-400/50"
          />
        </div>
        <button
          onClick={loadPreview}
          disabled={busy}
          className="px-4 py-2 rounded-lg bg-sky-500/15 text-sky-300 hover:bg-sky-500/25 text-sm font-medium flex items-center gap-2"
        >
          <RefreshCw size={14} className={busy ? "animate-spin" : ""} /> Xem trước (dry-run)
        </button>
        {preview && (
          <div className="ml-auto text-right">
            <div className="text-xs text-zinc-500">Ước tính nhận được (sau phí)</div>
            <div className="text-lg font-semibold text-emerald-400 tabular">
              {fmtUSD(preview.est_total_net)}
            </div>
            <div className="text-[11px] text-zinc-600">{tradableCount} coin bán được</div>
          </div>
        )}
      </div>

      {!preview && !busy && (
        <div className="panel p-10 text-center text-sm text-zinc-500">
          <Coins size={28} className="mx-auto mb-3 text-zinc-600" />
          Nhấn <b className="text-zinc-300">Xem trước</b> để liệt kê coin trên tài khoản
          testnet và ước tính USDT nhận được. Chưa đặt lệnh nào cả.
        </div>
      )}

      {preview && (
        <>
          <div className="flex items-center gap-3">
            <div className="relative flex-1 max-w-xs">
              <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
              <input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Lọc coin…"
                className="w-full bg-[#0d1117] border border-[#1e2430] rounded-lg pl-8 pr-3 py-2 text-sm text-white outline-none focus:border-amber-400/50"
              />
            </div>
            <span className="text-xs text-zinc-500">{rows.length} kết quả</span>
          </div>

          <div className="panel overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-zinc-500 border-b border-[#1e2430]">
                  <th className="text-left py-3 px-4 font-normal">Chọn</th>
                  <th className="text-left font-normal">Asset</th>
                  <th className="text-right font-normal">Qty (bán được)</th>
                  <th className="text-right font-normal">Giá (USDT)</th>
                  <th className="text-right font-normal">Giá trị</th>
                  <th className="text-right font-normal">Ước tính net</th>
                  <th className="text-right font-normal pr-4">Trạng thái</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr
                    key={r.asset}
                    className={`border-b border-[#161c28] hover:bg-white/[0.02] ${
                      !r.tradable ? "opacity-50" : ""
                    }`}
                  >
                    <td className="py-2 px-4">
                      <input
                        type="checkbox"
                        checked={picked.has(r.asset)}
                        onChange={() => toggle(r.asset)}
                        disabled={!r.tradable}
                        className="accent-amber-400"
                      />
                    </td>
                    <td className="text-white font-medium">{r.asset}</td>
                    <td className="text-right tabular text-zinc-300">{fmtQty(r.qty)}</td>
                    <td className="text-right tabular text-zinc-400">
                      {r.price ? fmtUSD(r.price, r.price >= 100 ? 2 : 4) : "—"}
                    </td>
                    <td className="text-right tabular text-zinc-300">{fmtUSD(r.usdt_value)}</td>
                    <td className="text-right tabular text-emerald-400/90">
                      {r.tradable ? fmtUSD(r.est_net) : "—"}
                    </td>
                    <td className="text-right pr-4">
                      {r.tradable ? (
                        <span className="px-1.5 py-0.5 rounded bg-emerald-500/15 text-emerald-300 text-[10px]">
                          sell
                        </span>
                      ) : (
                        <span
                          className="px-1.5 py-0.5 rounded bg-zinc-500/15 text-zinc-400 text-[10px]"
                          title={r.skip_reason}
                        >
                          {SKIP_LABEL[r.skip_reason] || "bỏ qua"}
                        </span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {rows.length === 0 && (
              <div className="py-10 text-center text-sm text-zinc-500">
                Không có coin nào khớp bộ lọc
              </div>
            )}
          </div>

          <button
            onClick={execute}
            disabled={busy || tradableCount === 0}
            className={`px-5 py-2.5 rounded-lg text-sm font-medium flex items-center gap-2 ${
              tradableCount > 0
                ? "bg-amber-400/15 text-amber-300 hover:bg-amber-400/25"
                : "bg-zinc-500/10 text-zinc-600 cursor-not-allowed"
            }`}
          >
            <ArrowRightLeft size={14} />
            {mode === "all"
              ? `Convert tất cả → USDT (${tradableCount} coin)`
              : `Convert ${[...picked].length} coin đã chọn → USDT`}
          </button>
        </>
      )}

      {/* results */}
      {execResults && (
        <div className="panel p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-medium text-white">Kết quả convert</h2>
            <span className="text-sm font-semibold text-emerald-400 tabular">
              +{fmtUSD(execResults.total_net_usdt)} USDT
            </span>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-zinc-500 border-b border-[#1e2430]">
                  <th className="text-left py-2 font-normal">Asset</th>
                  <th className="text-right font-normal">Qty</th>
                  <th className="text-right font-normal">Net (USDT)</th>
                  <th className="text-right font-normal pr-2">Kết quả</th>
                </tr>
              </thead>
              <tbody>
                {execResults.results.map((r, i) => (
                  <tr key={i} className="border-b border-[#161c28]">
                    <td className="py-1.5 text-white font-medium">{r.asset}</td>
                    <td className="text-right tabular text-zinc-300">{fmtQty(r.executed_qty ?? r.qty)}</td>
                    <td className="text-right tabular text-zinc-300">
                      {r.ok ? fmtUSD(r.net_usdt) : "—"}
                    </td>
                    <td className="text-right pr-2">
                      {r.ok ? (
                        <span className="text-emerald-400 text-[11px]">OK</span>
                      ) : (
                        <span className="text-rose-400 text-[11px]" title={r.error}>
                          lỗi
                        </span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
