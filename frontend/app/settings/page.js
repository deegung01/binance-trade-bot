"use client";

import { useEffect, useState } from "react";
import { Check, Eye, EyeOff, KeyRound, Save, Trash2 } from "lucide-react";
import { api } from "@/lib/api";

function Field({ label, hint, children }) {
  return (
    <div>
      <label className="text-xs text-zinc-500">{label}</label>
      {children}
      {hint && <p className="mt-1 text-[11px] text-zinc-600">{hint}</p>}
    </div>
  );
}

const inputCls =
  "mt-1 w-full bg-[#0d1117] border border-[#1e2430] rounded-lg px-3 py-2 text-sm text-white outline-none focus:border-amber-400/50";

export default function SettingsPage() {
  const [cfg, setCfg] = useState(null);
  const [creds, setCreds] = useState({ binance_api_key: "", binance_api_secret: "" });
  const [showSecret, setShowSecret] = useState(false);
  const [msg, setMsg] = useState({ type: "", text: "" });
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const c = await api("/config");
      setCfg(c);
    } catch (e) {
      setMsg({ type: "error", text: e.message });
    }
  };

  useEffect(() => {
    load();
  }, []);

  const save = async () => {
    setBusy(true);
    try {
      await api("/config", {
        method: "POST",
        body: JSON.stringify({
          trading_mode: cfg.trading_mode,
          paper_data_source: cfg.paper_data_source,
          start_balance: Number(cfg.start_balance),
          trading_symbols: cfg.trading_symbols,
          timeframe: cfg.timeframe,
          strategy: cfg.strategy,
          stake_mode: cfg.stake_mode,
          stake_amount: Number(cfg.stake_amount),
          stake_percent: Number(cfg.stake_percent),
          stop_loss_pct: Number(cfg.stop_loss_pct),
          take_profit_pct: Number(cfg.take_profit_pct),
          max_open_trades: Number(cfg.max_open_trades),
          poll_interval: Number(cfg.poll_interval),
          trailing_stop: !!cfg.trailing_stop,
          trailing_stop_pct: Number(cfg.trailing_stop_pct || 1),
          grid_levels: Number(cfg.grid_levels || 4),
        }),
      });
      setMsg({ type: "ok", text: "Saved — engine restarted with new config" });
    } catch (e) {
      setMsg({ type: "error", text: e.message });
    }
    setBusy(false);
  };

  const saveCreds = async () => {
    setBusy(true);
    try {
      await api("/credentials", {
        method: "POST",
        body: JSON.stringify(creds),
      });
      await load();
      setMsg({ type: "ok", text: "Credentials saved" });
    } catch (e) {
      setMsg({ type: "error", text: e.message });
    }
    setBusy(false);
  };

  const clearCreds = async () => {
    setBusy(true);
    try {
      await api("/credentials/clear", { method: "POST" });
      await load();
      setCreds({ binance_api_key: "", binance_api_secret: "" });
      setMsg({ type: "ok", text: "Credentials cleared — back to paper mode" });
    } catch (e) {
      setMsg({ type: "error", text: e.message });
    }
    setBusy(false);
  };

  const resetWallet = async () => {
    if (!confirm("Reset paper wallet? All trades/logs/equity history will be wiped.")) return;
    setBusy(true);
    try {
      await api("/reset", { method: "POST" });
      setMsg({ type: "ok", text: "Paper wallet reset to start balance" });
    } catch (e) {
      setMsg({ type: "error", text: e.message });
    }
    setBusy(false);
  };

  if (!cfg) return <div className="text-zinc-500 text-sm">Loading…</div>;

  return (
    <div className="space-y-6 max-w-[900px]">
      <div>
        <h1 className="text-xl font-semibold text-white">Settings</h1>
        <p className="text-sm text-zinc-500">Bot configuration — auto-applies on next engine cycle</p>
      </div>

      {msg.text && (
        <div className={`text-xs px-3 py-2 rounded-lg ${msg.type === "ok" ? "bg-emerald-500/10 text-emerald-300" : "bg-rose-500/10 text-rose-300"}`}>
          {msg.text}
        </div>
      )}

      {/* Trading */}
      <div className="panel p-5 space-y-4">
        <h2 className="text-sm font-medium text-white">Trading</h2>
        <div className="grid md:grid-cols-2 gap-4">
          <Field label="Mode">
            <select value={cfg.trading_mode} onChange={(e) => setCfg({ ...cfg, trading_mode: e.target.value })} className={inputCls}>
              <option value="paper">Paper (simulated wallet)</option>
              <option value="live">Live (Binance Spot Testnet orders)</option>
            </select>
          </Field>
          <Field label="Market data source" hint="Testnet prices are thin/unrealistic — mainnet gives real prices for paper trading">
            <select value={cfg.paper_data_source} onChange={(e) => setCfg({ ...cfg, paper_data_source: e.target.value })} className={inputCls}>
              <option value="testnet">Binance Testnet</option>
              <option value="mainnet">Binance Mainnet (prices only)</option>
            </select>
          </Field>
          <Field label="Symbols (comma-separated)">
            <input value={cfg.trading_symbols} onChange={(e) => setCfg({ ...cfg, trading_symbols: e.target.value.toUpperCase() })} className={inputCls} />
          </Field>
          <Field label="Timeframe">
            <select value={cfg.timeframe} onChange={(e) => setCfg({ ...cfg, timeframe: e.target.value })} className={inputCls}>
              {(cfg.timeframes || ["1m", "5m", "15m", "1h", "4h", "1d"]).map((t) => (
                <option key={t}>{t}</option>
              ))}
            </select>
          </Field>
          <Field label="Strategy">
            <select value={cfg.strategy} onChange={(e) => setCfg({ ...cfg, strategy: e.target.value })} className={inputCls}>
              {(cfg.strategies_available || []).map((s) => (
                <option key={s.id} value={s.id}>{s.label}</option>
              ))}
            </select>
          </Field>
          <Field label="Poll interval (seconds)" hint="How often the engine re-evaluates signals">
            <input type="number" value={cfg.poll_interval} onChange={(e) => setCfg({ ...cfg, poll_interval: e.target.value })} className={inputCls} />
          </Field>
        </div>
      </div>

      {/* Risk / stakes */}
      <div className="panel p-5 space-y-4">
        <h2 className="text-sm font-medium text-white">Risk & position sizing</h2>
        <div className="grid md:grid-cols-3 gap-4">
          <Field label="Start balance (USDT)" hint="Used when resetting paper wallet">
            <input type="number" value={cfg.start_balance} onChange={(e) => setCfg({ ...cfg, start_balance: e.target.value })} className={inputCls} />
          </Field>
          <Field label="Stake mode">
            <select value={cfg.stake_mode} onChange={(e) => setCfg({ ...cfg, stake_mode: e.target.value })} className={inputCls}>
              <option value="fixed">Fixed amount</option>
              <option value="percent">% of cash</option>
            </select>
          </Field>
          {cfg.stake_mode === "fixed" ? (
            <Field label="Stake per trade (USDT)">
              <input type="number" value={cfg.stake_amount} onChange={(e) => setCfg({ ...cfg, stake_amount: e.target.value })} className={inputCls} />
            </Field>
          ) : (
            <Field label="Stake per trade (% of cash)">
              <input type="number" value={cfg.stake_percent} onChange={(e) => setCfg({ ...cfg, stake_percent: e.target.value })} className={inputCls} />
            </Field>
          )}
          <Field label="Stop loss %">
            <input type="number" step="0.1" value={cfg.stop_loss_pct} onChange={(e) => setCfg({ ...cfg, stop_loss_pct: e.target.value })} className={inputCls} />
          </Field>
          <Field label="Take profit %">
            <input type="number" step="0.1" value={cfg.take_profit_pct} onChange={(e) => setCfg({ ...cfg, take_profit_pct: e.target.value })} className={inputCls} />
          </Field>
          <Field label="Max open trades">
            <input type="number" value={cfg.max_open_trades} onChange={(e) => setCfg({ ...cfg, max_open_trades: e.target.value })} className={inputCls} />
          </Field>
        </div>
      </div>

      {/* Trailing stop + Grid */}
      <div className="panel p-5 space-y-4">
        <h2 className="text-sm font-medium text-white">Trailing stop & Adaptive grid</h2>
        <div className="grid md:grid-cols-3 gap-4">
          <Field
            label="Trailing stop"
            hint="Bật để SL tự bám theo giá cao nhất (chỉ nâng lên, không hạ). Ap dụng cho MỌI strategy."
          >
            <select
              value={cfg.trailing_stop ? "on" : "off"}
              onChange={(e) => setCfg({ ...cfg, trailing_stop: e.target.value === "on" })}
              className={inputCls}
            >
              <option value="off">Tắt (chỉ SL/TP cố định)</option>
              <option value="on">Bật (SL trailing theo high)</option>
            </select>
          </Field>
          <Field
            label="Trailing distance (%)"
            hint="SL = highest_price × (1 − trail%). Kích hoạt khi lãi ≥ 2× khoảng cách"
          >
            <input
              type="number"
              step="0.1"
              value={cfg.trailing_stop_pct ?? 1}
              onChange={(e) => setCfg({ ...cfg, trailing_stop_pct: e.target.value })}
              className={inputCls}
            />
          </Field>
          <Field
            label="Grid levels (Adaptive Grid)"
            hint="Số lần DCA add tối đa mỗi lệnh khi chạy strategy Adaptive Grid"
          >
            <input
              type="number"
              value={cfg.grid_levels ?? 4}
              onChange={(e) => setCfg({ ...cfg, grid_levels: e.target.value })}
              className={inputCls}
            />
          </Field>
        </div>
      </div>

      <button
        onClick={save}
        disabled={busy}
        className="px-5 py-2.5 rounded-lg bg-amber-400/15 text-amber-300 hover:bg-amber-400/25 text-sm font-medium flex items-center gap-2"
      >
        <Save size={14} /> Save configuration
      </button>

      {/* Credentials */}
      <div className="panel p-5 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium text-white flex items-center gap-2">
            <KeyRound size={14} className="text-amber-400" /> Binance Testnet API keys
          </h2>
          {cfg.has_credentials && (
            <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/15 text-emerald-300">saved</span>
          )}
        </div>
        <p className="text-[11px] text-zinc-500">
          Get keys at <span className="text-amber-400">testnet.binance.vision</span> (login with GitHub). Only needed for LIVE mode — paper mode works without keys.
        </p>
        <div className="grid md:grid-cols-2 gap-4">
          <Field label="API key">
            <input
              value={creds.binance_api_key}
              onChange={(e) => setCreds({ ...creds, binance_api_key: e.target.value })}
              placeholder={cfg.has_credentials ? "•••••• saved — paste new to replace" : "64-char testnet API key"}
              className={inputCls}
            />
          </Field>
          <Field label="API secret">
            <div className="relative">
              <input
                type={showSecret ? "text" : "password"}
                value={creds.binance_api_secret}
                onChange={(e) => setCreds({ ...creds, binance_api_secret: e.target.value })}
                className={inputCls + " pr-10"}
              />
              <button
                onClick={() => setShowSecret(!showSecret)}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-white"
              >
                {showSecret ? <EyeOff size={14} /> : <Eye size={14} />}
              </button>
            </div>
          </Field>
        </div>
        <div className="flex gap-2">
          <button onClick={saveCreds} disabled={busy} className="px-4 py-2 rounded-lg bg-white/5 text-zinc-200 hover:bg-white/10 text-xs flex items-center gap-2">
            <Check size={13} /> Save keys
          </button>
          {cfg.has_credentials && (
            <button onClick={clearCreds} disabled={busy} className="px-4 py-2 rounded-lg bg-rose-500/10 text-rose-300 hover:bg-rose-500/20 text-xs flex items-center gap-2">
              <Trash2 size={13} /> Remove
            </button>
          )}
        </div>
      </div>

      {/* Danger zone */}
      <div className="panel p-5 border-rose-500/20">
        <h2 className="text-sm font-medium text-rose-300 mb-2">Danger zone</h2>
        <button
          onClick={resetWallet}
          disabled={busy}
          className="px-4 py-2 rounded-lg bg-rose-500/15 text-rose-300 hover:bg-rose-500/25 text-xs"
        >
          Reset paper wallet (wipe trades, logs, equity)
        </button>
      </div>
    </div>
  );
}
