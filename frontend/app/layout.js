import "./globals.css";
import { Providers } from "@/components/Providers";

export const metadata = {
  title: "TradeBot — Binance Testnet Dashboard",
  description: "Paper trading dashboard for Binance Spot Testnet",
};

export default function RootLayout({ children }) {
  return (
    <html lang="en">
      <head>
        <meta name="viewport" content="width=device-width, initial-scale=1" />
      </head>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}