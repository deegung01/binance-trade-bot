"use client";

import { useEffect, useState } from "react";
import { Bot, Zap } from "lucide-react";
import { api, fmtUSD } from "@/lib/api";

const STRATEGY_DOCS = {
  sma_cross: {
    name: "SMA Cross",
    desc: "Mua khi SMA nhanh (10) cắt lên SMA chậm (50) — golden cross. Thoát khi SMA nhanh cắt xuống.",
    rules: ["Entry: SMA10 > SMA50 (vừa cắt)", "Exit: SMA10 < SMA50", "Kèm SL/TP từ Settings"],
  },
  ema_cross: {
    name: "EMA Cross + RSI",
    desc: "EMA 9/21 cross với filter RSI > 50 (momentum xác nhận).",
    rules: ["Entry: EMA9 cắt lên EMA21 và RSI > 50", "Exit: EMA9 < EMA21"],
  },
  rsi_revert: {
    name: "RSI Reversion",
    desc: "Mean reversion: mua khi RSI vùng oversold hồi phục, thoát khi overbought.",
    rules: ["Entry: RSI cắt lên qua 30", "Exit: RSI ≥ 70"],
  },
  macd: {
    name: "MACD Flip",
    desc: "Mua khi histogram MACD翻 chuyển từ âm sang dương; thoát khi âm.",
    rules: ["Entry: hist chuyển từ ≤0 sang >0", "Exit: hist < 0"],
  },
  adaptive_grid: {
    name: "Adaptive Grid + DCA",
    desc: "Grid thích ứng theo volatility: spacing = ATR14% (kẹp 0.5%–4%). Giá tụt mỗi 1 spacing dưới giá vốn → DCA mua thêm, tối đa N levels. SL/TP tự rebase theo giá vốn trung bình mới. Kết hợp tốt với Trailing Stop.",
    rules: [
      "Entry: RSI 30–65 (thị trường đi ngang)",
      "Spacing động: clamp(ATR14% × 1.0, 0.5%, 4%)",
      "Add: giá ≤ avg_cost × (1 − spacing) → +1 grid stake",
      "Max levels: cấu hình Grid Levels ở Settings",
      "Exit: SL / TP / Trailing (engine quản lý)",
    ],
  },
};

export default function StrategyPage() {
  const [config, setConfig] = useState(null);
  const [msg, setMsg] = useState({ type: "", text: "" });
  const [manualSymbol, setManualSymbol] = useState("BTCUSDT");
  const [manualStake, setManualStake] = useState(100);
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const c = await api("/config");
      setConfig(c);
    } catch (e) {
      setMsg({ type: "error", text: e.message });
    }
  };

  useEffect(() => {
    load();
  }, []);

  const switchStrategy = async (id) => {
    setBusy(true);
    try {
      await api("/config", {
        method: "POST",
        body: JSON.stringify({ strategy: id }),
      });
      await load();
      setMsg({ type: "ok", text: `Switched to ${id}` });
    } catch (e) {
      setMsg({ type: "error", text: e.message });
    }
    setBusy(false);
  };

  const manualBuy = async () => {
    setBusy(true);
    try {
      const res = await api("/buy", {
        method: "POST",
        body: JSON.stringify({ symbol: manualSymbol, stake: Number(manualStake) }),
      });
      setMsg({ type: "ok", text: `Bought ${manualSymbol} @ ${fmtUSD(res.price)}` });
    } catch (e) {
      setMsg({ type: "error", text: e.message });
    }
    setBusy(false);
  };

  if (!config) return <div className="text-zinc-500 text-sm">Loading…</div>;

  const active = config.strategy;
  const symbols = (config.trading_symbols || "").split(",").map((s) => s.trim()).filter(Boolean);

  return (
    <div className="space-y-6 max-w-[1400px]">
      <div>
        <h1 className="text-xl font-semibold text-white">Strategy</h1>
        <p className="text-sm text-zinc-500">Active: <b className="text-amber-300">{active}</b> · timeframe {config.timeframe}</p>
      </div>

      {msg.text && (
        <div className={`text-xs px-3 py-2 rounded-lg ${msg.type === "ok" ? "bg-emerald-500/10 text-emerald-300" : "bg-rose-500/10 text-rose-300"}`}>
          {msg.text}
        </div>
      )}

      <div className="grid md:grid-cols-2 gap-4">
        {(config.strategies_available || []).map((s) => {
          const doc = STRATEGY_DOCS[s.id] || { name: s.label, desc: "", rules: [] };
          const isActive = s.id === active;
          return (
            <div
              key={s.id}
              className={`panel p-5 ${isActive ? "border-amber-400/40 bg-amber-400/[0.03]" : ""}`}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Bot size={16} className={isActive ? "text-amber-300" : "text-zinc-500"} />
                  <span className="font-medium text-white">{doc.name}</span>
                </div>
                {isActive ? (
                  <span className="text-[10px] px-2 py-0.5 rounded bg-amber-400/15 text-amber-300">ACTIVE</span>
                ) : (
                  <button
                    onClick={() => switchStrategy(s.id)}
                    disabled={busy}
                    className="text-[11px] px-2.5 py-1 rounded bg-white/5 text-zinc-300 hover:bg-white/10"
                  >
                    Switch to
                  </button>
                )}
              </div>
              <p className="mt-3 text-xs text-zinc-500 leading-relaxed">{doc.desc}</p>
              <ul className="mt-3 space-y-1">
                {doc.rules.map((r, i) => (
                  <li key={i} className="text-xs text-zinc-400 flex gap-2">
                    <Zap size={11} className="text-amber-400/70 mt-0.5 shrink-0" />
                    {r}
                  </li>
                ))}
              </ul>
            </div>
          );
        })}
      </div>

      {/* manual order */}
      <div className="panel p-5 max-w-md">
        <h2 className="text-sm font-medium text-white mb-4 flex items-center gap-2">
          <Zap size={14} className="text-amber-400" /> Manual order (market buy)
        </h2>
        <div className="space-y-3">
          <div>
            <label className="text-xs text-zinc-500">Symbol</label>
            <input
              list="symbol-list"
              value={manualSymbol}
              onChange={(e) => setManualSymbol(e.target.value.toUpperCase())}
              className="mt-1 w-full bg-[#0d1117] border border-[#1e2430] rounded-lg px-3 py-2 text-sm text-white outline-none focus:border-amber-400/50"
            />
            <datalist id="symbol-list">
              {symbols.map((s) => (
                <option key={s} value={s} />
              ))}
            </datalist>
          </div>
          <div>
            <label className="text-xs text-zinc-500">Stake (USDT)</label>
            <input
              type="number"
              value={manualStake}
              onChange={(e) => setManualStake(e.target.value)}
              className="mt-1 w-full bg-[#0d1117] border border-[#1e2430] rounded-lg px-3 py-2 text-sm text-white outline-none focus:border-amber-400/50"
            />
          </div>
          <button
            onClick={manualBuy}
            disabled={busy}
            className="w-full py-2.5 rounded-lg bg-emerald-500/15 text-emerald-300 hover:bg-emerald-500/25 text-sm font-medium"
          >
            Buy {manualSymbol} · {manualStake} USDT
          </button>
          <p className="text-[11px] text-zinc-600">
            Market buy bằng giá testnet hiện tại. Đóng lệnh ở trang Trades.
          </p>
        </div>
      </div>
    </div>
  );
}
