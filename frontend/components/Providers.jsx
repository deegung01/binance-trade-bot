"use client";

import { ToastProvider, SidebarProvider } from "@/components/Toast";
import Sidebar from "@/components/Sidebar";

export function Providers({ children }) {
  return (
    <SidebarProvider>
      <ToastProvider>
        <Sidebar />
        <main className="min-h-screen p-4 sm:p-6 pb-24 lg:pb-8 lg:pl-60 lg:pr-8 lg:pt-8">
          {children}
        </main>
      </ToastProvider>
    </SidebarProvider>
  );
}