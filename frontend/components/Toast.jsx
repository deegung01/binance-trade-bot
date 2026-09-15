"use client";

import { createContext, useContext, useState } from "react";
import { CheckCircle2, AlertTriangle, Info, X } from "lucide-react";

const ToastCtx = createContext(null);
const SidebarCtx = createContext(null);

let idSeq = 1;

export function ToastProvider({ children }) {
  const [toasts, setToasts] = useState([]);

  const dismiss = (id) => {
    setToasts((t) => t.filter((x) => x.id !== id));
  };

  const push = (type, text, ms = 4000) => {
    const id = idSeq++;
    setToasts((t) => [...t, { id, type, text }]);
    if (ms > 0) {
      setTimeout(() => {
        setToasts((t) => t.filter((x) => x.id !== id));
      }, ms);
    }
    return id;
  };

  const toast = {
    ok: (text, ms) => push("ok", text, ms),
    err: (text, ms) => push("err", text, ms ?? 7000),
    info: (text, ms) => push("info", text, ms),
  };

  return (
    <ToastCtx.Provider value={{ toast, dismiss }}>
      {children}
      <div className="fixed bottom-4 right-4 z-[100] flex flex-col gap-2 max-w-[380px]">
        {toasts.map((t) => (
          <div
            key={t.id}
            role="status"
            className={`panel px-4 py-3 flex items-start gap-2.5 text-sm shadow-lg animate-[slidein_.2s_ease-out] ${
              t.type === "ok"
                ? "border-emerald-500/40"
                : t.type === "err"
                ? "border-rose-500/40"
                : "border-sky-500/40"
            }`}
          >
            {t.type === "ok" && <CheckCircle2 size={16} className="text-emerald-400 shrink-0 mt-0.5" />}
            {t.type === "err" && <AlertTriangle size={16} className="text-rose-400 shrink-0 mt-0.5" />}
            {t.type === "info" && <Info size={16} className="text-sky-400 shrink-0 mt-0.5" />}
            <span className={`break-all ${t.type === "err" ? "text-rose-200" : t.type === "ok" ? "text-emerald-100" : "text-sky-100"}`}>
              {t.text}
            </span>
            <button
              onClick={() => dismiss(t.id)}
              className="ml-auto text-zinc-500 hover:text-white shrink-0"
              aria-label="Dismiss"
            >
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export function useToast() {
  return useContext(ToastCtx);
}

// Sidebar mobile state context
export function SidebarProvider({ children }) {
  const [mobileOpen, setMobileOpen] = useState(false);
  return (
    <SidebarCtx.Provider value={{ mobileOpen, setMobileOpen }}>
      {children}
    </SidebarCtx.Provider>
  );
}

export function useSidebar() {
  return useContext(SidebarCtx);
}