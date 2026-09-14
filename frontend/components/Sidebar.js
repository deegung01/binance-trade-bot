"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Activity,
  ArrowRightLeft,
  BarChart3,
  Bot,
  FileText,
  LayoutDashboard,
  Settings,
  Shield,
  Wallet,
} from "lucide-react";

const NAV = [
  { href: "/", label: "Overview", icon: LayoutDashboard },
  { href: "/chart", label: "Chart", icon: BarChart3 },
  { href: "/trades", label: "Trades", icon: Wallet },
  { href: "/strategy", label: "Strategy", icon: Bot },
  { href: "/convert", label: "Convert → USDT", icon: ArrowRightLeft },
  { href: "/logs", label: "Logs", icon: FileText },
  { href: "/settings", label: "Settings", icon: Settings },
];

export default function Sidebar() {
  const pathname = usePathname();
  const [status, setStatus] = useState(null);

  useEffect(() => {
    let alive = true;
    const load = async () => {
      try {
        const res = await fetch(
          `${process.env.NEXT_PUBLIC_API_URL || "http://127.0.0.1:8000"}/api/status`
        );
        if (res.ok) {
          const j = await res.json();
          if (alive) setStatus(j);
        }
      } catch {}
    };
    load();
    const t = setInterval(load, 10000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  const running = status?.running ?? false;
  const mode = status?.mode ?? "paper";

  return (
    <aside className="fixed left-0 top-0 bottom-0 w-56 bg-[#0d1117] border-r border-[#1e2430] flex flex-col z-40">
      <div className="px-5 py-5 border-b border-[#1e2430]">
        <div className="flex items-center gap-2">
          <Bot className="text-amber-400" size={22} />
          <div>
            <div className="font-semibold text-white text-sm leading-tight">TradeBot</div>
            <div className="text-[10px] text-zinc-500">BINANCE TESTNET</div>
          </div>
        </div>
      </div>

      <nav className="flex-1 px-3 py-4 space-y-1">
        {NAV.map((item) => {
          const Icon = item.icon;
          const active = pathname === item.href;
          return (
            <Link
              key={item.href}
              href={item.href}
              className={`flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors ${
                active
                  ? "bg-amber-400/10 text-amber-300"
                  : "text-zinc-400 hover:text-zinc-200 hover:bg-white/5"
              }`}
            >
              <Icon size={16} />
              {item.label}
            </Link>
          );
        })}
      </nav>

      <div className="px-4 py-4 border-t border-[#1e2430] space-y-3">
        <div className="flex items-center gap-2 text-xs">
          <span
            className={`inline-block w-2 h-2 rounded-full ${
              running ? "bg-emerald-400 live-dot" : "bg-zinc-600"
            }`}
          />
          <span className="text-zinc-400">{running ? "Running" : "Paused"}</span>
          <span
            className={`ml-auto text-[10px] px-1.5 py-0.5 rounded ${
              mode === "live"
                ? "bg-rose-500/15 text-rose-300"
                : "bg-sky-500/15 text-sky-300"
            }`}
          >
            {mode === "live" ? "LIVE TESTNET" : "PAPER"}
          </span>
        </div>
        <div className="flex items-center gap-2 text-[10px] text-zinc-500">
          <Shield size={12} />
          Sandbox only — no real funds
        </div>
      </div>
    </aside>
  );
}
