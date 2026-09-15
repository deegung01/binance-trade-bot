"use client";

import { useEffect, useState, useMemo } from "react";
import { KeyRound, Save, ShieldAlert, Bot, Timer, ChevronDown, AlertTriangle } from "lucide-react";
import { api, fmtUSD, fmtPct, fmtNum } from "@/lib/api";
import { useToast } from "@/components/Toast";
import EquityChart from "@/components/EquityChart";

const inputCls = "input";
const selectCls = "select";

function Field({ label, hint, children }) {
  return (
    <div className="space-y-1">
      <div className="label">{label}</div>
      {children}
      {hint && <p className="hint">{hint}</p>}
    </div>
  );
}

function Accordion({ title, icon: Icon, children, defaultOpen = true }) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className="card overflow-hidden">
      <button className="card-header w-full flex items-center justify-between hover:bg-white/5 transition-colors" onClick={() => setOpen(!open)}>
        <div className="flex items-center gap-2">
          {Icon && <Icon size={16} className="text-amber-400" />}
          <span className="font-medium text-white">{title}</span>
        </div>
        <ChevronDown size={16} className={`accordion-icon text-zinc-500 ${open ? "rotate-180" : ""}`} />
      </button>
      {open && (
        <div className="card-body border-t border-[#1e2430] animate-[slidein_.2s_ease-out]">
          {children}
        </div>
      )}
    </div>
  );
}

export default function SettingsPage() {
  const toast = useToast();
  const [cfg, setCfg] = useState(null);
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

  if (!cfg) return <div className="empty h-64"><div className="empty-icon"><div className="skeleton w-12 h-12 rounded-full" /></div><div className="empty-title">Loading…</div></div>;

  return (
    <div className="container section">
      <div className="page-header">
        <div>
          <h1 className="page-title">Settings</h1>
          <p className="page-subtitle">Cấu hình bot — tự áp dụng ở engine cycle kế tiếp</p>
        </div>
        <button onClick={save} disabled={busy} className="btn-primary">
          <Save size={14} /> {busy ? "Đang lưu…" : "Lưu cấu hình"}
        </button>
      </div>

      <div className="grid-auto-lg gap-4">
        <Accordion title="Trading" icon={Bot} defaultOpen={true}>
          <div className="grid-auto-sm lg:grid-cols-4">
            <Field label="Mode" hint="Paper = ví mô phỏng, Live = lệnh thật trên testnet">
              <select value={cfg.trading_mode} onChange={(e) => setCfg({ ...cfg, trading_mode: e.target.value })} className={selectCls}>
                <option value="paper">Paper (ví mô phỏng)</option>
                <option value="live">Live (lệnh thật trên testnet)</option>
              </select>
            </Field>
            <Field label="Nguồn dữ liệu giá" hint="Giá testnet mỏng & thiếu thực tế — mainnet cho giá thật khi paper trading">
              <select value={cfg.paper_data_source} onChange={(e) => setCfg({ ...cfg, paper_data_source: e.target.value })} className={selectCls}>
                <option value="testnet">Binance Testnet</option>
                <option value="mainnet">Binance Mainnet (chỉ giá)</option>
              </select>
            </Field>
            <Field label="Symbols (phẩy cách nhau)">
              <input value={cfg.trading_symbols} onChange={(e) => setCfg({ ...cfg, trading_symbols: e.target.value.toUpperCase() })} className={inputCls} />
            </Field>
            <Field label="Timeframe">
              <select value={cfg.timeframe} onChange={(e) => setCfg({ ...cfg, timeframe: e.target.value })} className={selectCls}>
                {(cfg.timeframes || ["1m", "5m", "15m", "1h", "4h", "1d"]).map((t) => (
                  <option key={t}>{t}</option>
                ))}
              </select>
            </Field>
            <Field label="Strategy">
              <select value={cfg.strategy} onChange={(e) => setCfg({ ...cfg, strategy: e.target.value })} className={selectCls}>
                {(cfg.strategies_available || []).map((s) => (
                  <option key={s.id} value={s.id}>{s.label}</option>
                ))}
              </select>
            </Field>
            <Field label="Poll interval (giây)" hint="Engine re-evaluate signals bao lâu một lần">
              <input type="number" value={cfg.poll_interval} onChange={(e) => setCfg({ ...cfg, poll_interval: e.target.value })} className={inputCls} />
            </Field>
          </div>
        </Accordion>

        <Accordion title="Risk & Sizing" icon={ShieldAlert} defaultOpen={true}>
          <div className="grid-auto-sm lg:grid-cols-4">
            <Field label="Start balance (USDT)" hint="Dùng khi reset paper wallet">
              <input type="number" value={cfg.start_balance} onChange={(e) => setCfg({ ...cfg, start_balance: e.target.value })} className={inputCls} />
            </Field>
            <Field label="Stake mode">
              <select value={cfg.stake_mode} onChange={(e) => setCfg({ ...cfg, stake_mode: e.target.value })} className={selectCls}>
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
            <Field label="Trailing stop" hint="Bật để SL tự bám theo giá cao nhất (chỉ nâng lên). Áp dụng MỌI strategy.">
              <select value={cfg.trailing_stop ? "on" : "off"} onChange={(e) => setCfg({ ...cfg, trailing_stop: e.target.value === "on" })} className={selectCls}>
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
        </Accordion>

        <Accordion title="Risk Guards" icon={ShieldAlert} defaultOpen={true}>
          <div className="grid-auto-sm lg:grid-cols-3">
            <Field label="Cooldown sau stop loss (phút)" hint="Symbol vừa bị stop_loss phải chờ N phút trước khi vào lệnh lại. 0 = tắt. Tránh revenge-trade ngay sau khi cắt lỗ.">
              <input type="number" min="0" value={cfg.cooldown_minutes ?? 0} onChange={(e) => setCfg({ ...cfg, cooldown_minutes: e.target.value })} className={inputCls} />
            </Field>
            <Field label="Daily loss limit (% equity)" hint="Mất ≥ X% trong ngày (UTC) → bot tự PAUSE. 0 = tắt. Circuit breaker chống thua đây.">
              <input type="number" step="0.5" min="0" value={cfg.daily_loss_limit_pct ?? 0} onChange={(e) => setCfg({ ...cfg, daily_loss_limit_pct: e.target.value })} className={inputCls} />
            </Field>
            <Field label="Correlation block" hint="ρ ≥ 0.85 với vị thế đang mở → chặn entry mới. Tránh gộp rủi ro BTC+ETH+SOL cùng lúc. (Hardcoded)">
              <div className="input bg-zinc-800 text-zinc-400 cursor-not-allowed" style={{userSelect: 'none'}}>0.85</div>
            </Field>
          </div>
          <p className="hint flex items-start gap-2">
            <Timer size={12} className="mt-0.5 shrink-0" />
            Cooldown áp cho stop_loss / trailing_stop / regime cut. Daily limit so equity đầu ngày UTC với equity hiện tại — khi chạm limit, bot pause và bạn phải bật lại thủ công. Correlation filter chạy mỗi 10 phút từ candles đã fetch.
          </p>
        </Accordion>

        <Accordion title="API Keys" icon={KeyRound} defaultOpen={false}>
          <div className="flex items-center justify-between mb-2">
            <span className="font-medium text-white">Binance Testnet API keys</span>
            {cfg.has_credentials && <span className="badge-ok">đã cấu hình qua env</span>}
          </div>
          <p className="hint">Backend (Go) đọc keys từ biến môi trường — không lưu trong database. Lấy keys tại <span className="text-amber-400">testnet.binance.vision</span> (login GitHub), rồi set trên Render:</p>
          <div className="rounded-lg bg-[#0d1117] border border-[#1e2430] p-3 font-mono text-[11px] text-zinc-300 space-y-1">
            <p className="text-zinc-500"># Render → Service → Environment:</p>
            <p>{`BINANCE_API_KEY = <64-char key>`}</p>
            <p>{`BINANCE_API_SECRET = <secret>`}</p>
            <p className="text-zinc-500"># sau đó set TRADING_MODE=live để bot đặt lệnh thật trên testnet</p>
          </div>
          <p className="hint mt-2">
            Paper mode hoạt động không cần keys. Mode hiện tại:{" "}
            <b className={cfg.trading_mode === "live" ? "text-rose-300" : "text-sky-300"}>{cfg.trading_mode}</b>
          </p>
        </Accordion>

        <Accordion title="Danger Zone" icon={AlertTriangle} defaultOpen={false}>
          <div className="border-t border-[#1e2430] pt-4">
            <button onClick={resetWallet} disabled={busy} className="btn-danger w-full">
              Reset paper wallet (xóa trades, logs, equity)
            </button>
          </div>
        </Accordion>
      </div>
    </div>
  );
}