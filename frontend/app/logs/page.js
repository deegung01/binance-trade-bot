"use client";

import { useEffect, useState } from "react";
import { AlertTriangle, Info, RefreshCw } from "lucide-react";
import { api, fmtTime } from "@/lib/api";

const LEVELS = ["ALL", "INFO", "WARN", "ERROR"];

export default function LogsPage() {
  const [logs, setLogs] = useState([]);
  const [activity, setActivity] = useState([]);
  const [level, setLevel] = useState("ALL");
  const [err, setErr] = useState("");

  const load = async () => {
    try {
      const [l, a] = await Promise.all([
        api(`/logs?limit=${level === "ALL" ? 200 : 200}&level=${level === "ALL" ? "" : level}`),
        api("/activity?limit=50"),
      ]);
      setLogs(l.logs);
      setActivity(a.activity);
      setErr("");
    } catch (e) {
      setErr(e.message);
    }
  };

  useEffect(() => {
    load();
    const t = setInterval(load, 5000);
    return () => clearInterval(t);
  }, [level]);

  const icon = (lvl) =>
    lvl === "ERROR" ? <AlertTriangle size={12} className="text-rose-400" /> :
    lvl === "WARN" ? <AlertTriangle size={12} className="text-amber-400" /> :
    <Info size={12} className="text-sky-400" />;

  return (
    <div className="space-y-4 max-w-[1400px]">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-white">Logs</h1>
          <p className="text-sm text-zinc-500">Engine logs + order activity</p>
        </div>
        <div className="flex gap-2 items-center">
          <div className="flex rounded-lg overflow-hidden border border-[#1e2430] text-xs">
            {LEVELS.map((l) => (
              <button
                key={l}
                onClick={() => setLevel(l)}
                className={`px-3 py-1.5 ${
                  level === l ? "bg-amber-400/15 text-amber-300" : "text-zinc-400 hover:text-white"
                }`}
              >
                {l}
              </button>
            ))}
          </div>
          <button onClick={load} className="panel px-3 py-1.5 text-sm text-zinc-300 hover:text-white flex items-center gap-2">
            <RefreshCw size={13} /> Refresh
          </button>
        </div>
      </div>

      {err && <div className="text-xs text-rose-400">{err}</div>}

      <div className="grid lg:grid-cols-2 gap-4">
        {/* engine logs */}
        <div className="panel p-4">
          <h2 className="text-sm font-medium text-white mb-3">Engine logs</h2>
          <div className="space-y-1.5 max-h-[480px] overflow-y-auto font-mono text-[11px]">
            {logs.map((l) => (
              <div key={l.id} className="flex gap-2 items-start border-b border-[#161c28] pb-1.5">
                <span className="text-zinc-600 shrink-0">{fmtTime(l.created_at)}</span>
                {icon(l.level)}
                <span className="text-zinc-300 break-all">{l.message}</span>
              </div>
            ))}
            {logs.length === 0 && <p className="text-zinc-500 text-xs py-4 text-center">No logs</p>}
          </div>
        </div>

        {/* order activity */}
        <div className="panel p-4">
          <h2 className="text-sm font-medium text-white mb-3">Order activity</h2>
          <div className="space-y-1.5 max-h-[480px] overflow-y-auto font-mono text-[11px]">
            {activity.map((a) => (
              <div key={a.id} className="flex gap-2 items-start border-b border-[#161c28] pb-1.5">
                <span className="text-zinc-600 shrink-0">{fmtTime(a.created_at)}</span>
                <span
                  className={`shrink-0 ${
                    a.action === "buy" ? "text-emerald-400" :
                    a.action === "sell" ? "text-rose-400" :
                    a.action === "error" ? "text-rose-300" : "text-zinc-500"
                  }`}
                >
                  {a.action.toUpperCase()}
                </span>
                <span className="text-zinc-300">{a.symbol}</span>
                <span className="text-zinc-500 break-all">{a.detail}</span>
              </div>
            ))}
            {activity.length === 0 && <p className="text-zinc-500 text-xs py-4 text-center">No activity</p>}
          </div>
        </div>
      </div>
    </div>
  );
}
