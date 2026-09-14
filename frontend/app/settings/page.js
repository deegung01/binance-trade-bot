"use client";

import { useEffect, useState } from "react";
import { KeyRound, Save, ShieldAlert, Timer } from "lucide-react";
import { api } from "@/lib/api";
import { useToast } from "@/components/Toast";

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

const TABS = [
  { id: "trading", label: "Trading" },
  { id: "risk", label: "Risk & Sizing" },
  { id: "guards", label: "Risk Guards" },
  { id: "api", label: "API Keys" },
  { id: "danger", label: "Danger Zone" },
];

export default function SettingsPage() {
  const toast = useToast();
  const [cfg, setCfg] = useState(null);
  const [tab, setTab] = useState("trading");
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const c = await api("/config");
      setCfg(c);
    } catch (e) {
      toast.err(e.message);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
          cooldown_minutes: Number(cfg.cooldown_minutes || 0),
          daily_loss_limit_pct: Number(cfg.daily_loss_limit_pct || 0),
        }),
      });
      toast.ok("Đã lưu — áp dụng từ cycle sau");
    } catch (e) {
      toast.err(e.message);
    }
    setBusy(false);
  };

  const resetWallet = async () => {
    if (!confirm("Reset paper wallet? Toàn bộ trades/logs/equity sẽ bị xóa.")) return;
    setBusy(true);
    try {
      await api("/reset", { method: "POST" });
      toast.ok("Paper wallet đã reset về start balance");
    } catch (e) {
      toast.err(e.message);
    }
    setBusy(false);
  };

  if (!cfg) return <div className="text-zinc-500 text-sm">Loading…</div>;

  return (
    <div className="space-y-6 max-w-[900px]">
      <div>
        <h1 className="text-xl font-semibold text-white">Settings</h1>
        <p className="text-sm text-zinc-500">Cấu hình bot — tự áp dụng ở engine cycle kế tiếp</p>
      </div>

      {/* tabs */}
      <div className="flex gap-1 border-b border-[#1e2430]">
        {TABS.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`px-4 py-2 text-sm rounded-t-lg transition-colors ${
              tab === t.id
                ? "text-amber-300 bg-amber-400/10 border-b-2 border-amber-400"
                : "text-zinc-400 hover:text-white"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* TRADING */}
      {tab === "trading" && (
        <div className="panel p-5 space-y-4">
          <h2 className="text-sm font-medium text-white">Trading</h2>
          <div className="grid md:grid-cols-2 gap-4">
            <Field label="Mode">
              <select value={cfg.trading_mode} onChange={(e) => setCfg({ ...cfg, trading_mode: e.target.value })} className={inputCls}>
                <option value="paper">Paper (ví mô phỏng)</option>
                <option value="live">Live (lệnh thật trên testnet)</option>
              </select>
            </Field>
            <Field label="Nguồn dữ liệu giá" hint="Giá testnet mỏng & thiếu thực tế — mainnet cho giá thật khi paper trading">
              <select value={cfg.paper_data_source} onChange={(e) => setCfg({ ...cfg, paper_data_source: e.target.value })} className={inputCls}>
                <option value="testnet">Binance Testnet</option>
                <option value="mainnet">Binance Mainnet (chỉ giá)</option>
              </select>
            </Field>
            <Field label="Symbols (cách nhau bằng dấu phẩy)">
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
            <Field label="Poll interval (giây)" hint="Engine re-evaluate signals bao lâu một lần">
              <input type="number" value={cfg.poll_interval} onChange={(e) => setCfg({ ...cfg, poll_interval: e.target.value })} className={inputCls} />
            </Field>
          </div>
        </div>
      )}

      {/* RISK & SIZING */}
      {tab === "risk" && (
        <div className="panel p-5 space-y-4">
          <h2 className="text-sm font-medium text-white">Risk & position sizing</h2>
          <div className="grid md:grid-cols-3 gap-4">
            <Field label="Start balance (USDT)" hint="Dùng khi reset paper wallet">
              <input type="number" value={cfg.start_balance} onChange={(e) => setCfg({ ...cfg, start_balance: e.target.value })} className={inputCls} />
            </Field>
            <Field label="Stake mode">
              <select value={cfg.stake_mode} onChange={(e) => setCfg({ ...cfg, stake_mode: e.target.value })} className={inputCls}>
                <option value="fixed">Số cố định</option>
                <option value="percent">% của cash</option>
              </select>
            </Field>
            {cfg.stake_mode === "fixed" ? (
              <Field label="Stake mỗi lệnh (USDT)">
                <input type="number" value={cfg.stake_amount} onChange={(e) => setCfg({ ...cfg, stake_amount: e.target.value })} className={inputCls} />
              </Field>
            ) : (
              <Field label="Stake mỗi lệnh (% cash)">
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
            <Field
              label="Trailing stop"
              hint="Bật để SL tự bám theo giá cao nhất (chỉ nâng lên). Áp dụng MỌI strategy."
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
            <Field label="Trailing distance (%)" hint="SL = highest × (1 − trail%). Kích hoạt khi lãi ≥ 2× khoảng cách">
              <input type="number" step="0.1" value={cfg.trailing_stop_pct ?? 1} onChange={(e) => setCfg({ ...cfg, trailing_stop_pct: e.target.value })} className={inputCls} />
            </Field>
            <Field label="Grid levels (Adaptive Grid)" hint="Số lần DCA add tối đa mỗi lệnh">
              <input type="number" value={cfg.grid_levels ?? 4} onChange={(e) => setCfg({ ...cfg, grid_levels: e.target.value })} className={inputCls} />
            </Field>
          </div>
        </div>
      )}

      {/* RISK GUARDS */}
      {tab === "guards" && (
        <div className="panel p-5 space-y-4">
          <h2 className="text-sm font-medium text-white flex items-center gap-2">
            <ShieldAlert size={14} className="text-amber-400" /> Risk guards
          </h2>
          <div className="grid md:grid-cols-2 gap-4">
            <Field
              label="Cooldown sau stop loss (phút)"
              hint="Symbol vừa bị stop_loss phải chờ N phút trước khi vào lệnh lại. 0 = tắt. Tránh revenge-trade ngay sau khi cắt lỗ."
            >
              <input type="number" min="0" value={cfg.cooldown_minutes ?? 0} onChange={(e) => setCfg({ ...cfg, cooldown_minutes: e.target.value })} className={inputCls} />
            </Field>
            <Field
              label="Daily loss limit (% equity)"
              hint="Mất ≥ X% trong ngày (UTC) → bot tự PAUSE. 0 = tắt. Circuit breaker chống thua dây."
            >
              <input type="number" step="0.5" min="0" value={cfg.daily_loss_limit_pct ?? 0} onChange={(e) => setCfg({ ...cfg, daily_loss_limit_pct: e.target.value })} className={inputCls} />
            </Field>
          </div>
          <div className="text-[11px] text-zinc-600 flex items-start gap-2 pt-1">
            <Timer size={12} className="mt-0.5 shrink-0" />
            Cooldown áp cho stop_loss / trailing_stop / regime cut. Daily limit so equity đầu
            ngày UTC với equity hiện tại — khi chạm limit, bot pause và bạn phải bật lại thủ công.
          </div>
        </div>
      )}

      {/* API KEYS */}
      {tab === "api" && (
        <div className="panel p-5 space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-medium text-white flex items-center gap-2">
              <KeyRound size={14} className="text-amber-400" /> Binance Testnet API keys
            </h2>
            {cfg.has_credentials && (
              <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/15 text-emerald-300">đã cấu hình qua env</span>
            )}
          </div>
          <p className="text-[11px] text-zinc-500">
            Backend (Go) đọc keys từ biến môi trường — không lưu trong database. Lấy keys tại{" "}
            <span className="text-amber-400">testnet.binance.vision</span> (login GitHub), rồi set trên Render:
          </p>
          <div className="rounded-lg bg-[#0d1117] border border-[#1e2430] p-3 font-mono text-[11px] text-zinc-300 space-y-1">
            <p className="text-zinc-500"># Render → Service → Environment:</p>
            <p>BINANCE_API_KEY = &lt;64-char key&gt;</p>
            <p>BINANCE_API_SECRET = &lt;secret&gt;</p>
            <p className="text-zinc-500"># sau đó set TRADING_MODE=live để bot đặt lệnh thật trên testnet</p>
          </div>
          <p className="text-[11px] text-zinc-600">
            Paper mode hoạt động không cần keys. Mode hiện tại:{" "}
            <b className={cfg.trading_mode === "live" ? "text-rose-300" : "text-sky-300"}>{cfg.trading_mode}</b>
          </p>
        </div>
      )}

      {/* DANGER */}
      {tab === "danger" && (
        <div className="panel p-5 border-rose-500/20">
          <h2 className="text-sm font-medium text-rose-300 mb-2">Danger zone</h2>
          <button
            onClick={resetWallet}
            disabled={busy}
            className="px-4 py-2 rounded-lg bg-rose-500/15 text-rose-300 hover:bg-rose-500/25 text-xs"
          >
            Reset paper wallet (xóa trades, logs, equity)
          </button>
        </div>
      )}

      {tab !== "danger" && tab !== "api" && (
        <button
          onClick={save}
          disabled={busy}
          className="px-5 py-2.5 rounded-lg bg-amber-400/15 text-amber-300 hover:bg-amber-400/25 text-sm font-medium flex items-center gap-2"
        >
          <Save size={14} /> Lưu cấu hình
        </button>
      )}
    </div>
  );
}
