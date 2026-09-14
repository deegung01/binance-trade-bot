import "./globals.css";
import Sidebar from "@/components/Sidebar";
import { ToastProvider } from "@/components/Toast";

export const metadata = {
  title: "TradeBot — Binance Testnet Dashboard",
  description: "Paper trading dashboard for Binance Spot Testnet",
};

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <body>
        <ToastProvider>
          <Sidebar />
          <main className="ml-56 min-h-screen p-6">{children}</main>
        </ToastProvider>
      </body>
    </html>
  );
}
