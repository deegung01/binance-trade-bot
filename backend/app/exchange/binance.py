"""
Binance exchange client.

- `BinanceClient`: public market data (klines, prices) + signed private
  requests (account, orders) against https://testnet.binance.vision.
- `PaperClient`: simulated wallet, real market data. Mirrors the same
  interface so the engine code doesn't care which one it talks to.
"""
from __future__ import annotations

import hashlib
import hmac
import time
from typing import Any, Dict, List, Optional

import httpx

TESTNET_BASE = "https://testnet.binance.vision/api/v3"
MAINNET_BASE = "https://api.binance.com/api/v3"

FEE_RATE = 0.001  # 0.1% per side, mirrors Binance spot default


class ExchangeError(Exception):
    pass


def _utc_ms() -> int:
    return int(time.time() * 1000)


class BinanceClient:
    """Real REST client for Binance Spot Testnet."""

    def __init__(self, api_key: str = "", api_secret: str = "", base_url: str = TESTNET_BASE):
        self.api_key = api_key
        self.api_secret = api_secret
        self.base = base_url
        self._http = httpx.Client(timeout=15)

    # ---- public ----
    def ping(self) -> bool:
        try:
            r = self._http.get(f"{self.base}/ping")
            return r.status_code == 200
        except httpx.HTTPError:
            return False

    def server_time(self) -> int:
        r = self._http.get(f"{self.base}/time")
        r.raise_for_status()
        return r.json()["serverTime"]

    def klines(self, symbol: str, interval: str, limit: int = 200) -> List[Dict[str, Any]]:
        r = self._http.get(
            f"{self.base}/klines",
            params={"symbol": symbol, "interval": interval, "limit": limit},
        )
        if r.status_code != 200:
            raise ExchangeError(f"klines {symbol} failed: {r.status_code} {r.text[:200]}")
        rows = r.json()
        out = []
        for k in rows:
            out.append(
                {
                    "open_time": k[0],
                    "open": float(k[1]),
                    "high": float(k[2]),
                    "low": float(k[3]),
                    "close": float(k[4]),
                    "volume": float(k[5]),
                    "close_time": k[6],
                    "trades": k[8],
                }
            )
        return out

    def ticker_price(self, symbol: str) -> float:
        r = self._http.get(f"{self.base}/ticker/price", params={"symbol": symbol})
        if r.status_code != 200:
            raise ExchangeError(f"ticker {symbol} failed: {r.status_code} {r.text[:200]}")
        return float(r.json()["price"])

    def exchange_info(self) -> List[dict]:
        r = self._http.get(f"{self.base}/exchangeInfo")
        r.raise_for_status()
        return [
            {
                "symbol": s["symbol"],
                "base": s["baseAsset"],
                "quote": s["quoteAsset"],
                "status": s.get("status", ""),
                "filters": {
                    f["filterType"]: {k: v for k, v in f.items() if k != "filterType"}
                    for f in s.get("filters", [])
                },
            }
            for s in r.json().get("symbols", [])
        ]

    # ---- signed ----
    def _signed_params(self, params: Dict[str, Any]) -> Dict[str, Any]:
        params = dict(params)
        params["timestamp"] = _utc_ms()
        params["recvWindow"] = 10000
        qs = "&".join(f"{k}={v}" for k, v in params.items())
        sig = hmac.new(
            self.api_secret.encode(), qs.encode(), hashlib.sha256
        ).hexdigest()
        params["signature"] = sig
        return params

    def _signed_request(self, method: str, path: str, params: Optional[dict] = None) -> Any:
        if not self.api_key or not self.api_secret:
            raise ExchangeError("Missing API credentials for signed request")
        params = self._signed_params(params or {})
        r = self._http.request(
            method,
            f"{self.base}{path}",
            params=params,
            headers={"X-MBX-APIKEY": self.api_key},
        )
        if r.status_code != 200:
            raise ExchangeError(f"{path} failed: {r.status_code} {r.text[:300]}")
        return r.json()

    def account(self) -> dict:
        data = self._signed_request("GET", "/account")
        return {
            "can_trade": data.get("canTrade", True),
            "balances": [
                {"asset": b["asset"], "free": float(b["free"]), "locked": float(b["locked"])}
                for b in data.get("balances", [])
                if float(b["free"]) > 0 or float(b["locked"]) > 0
            ],
        }

    def market_buy(self, symbol: str, quote_qty: float) -> dict:
        """MARKET buy by quote amount (quoteOrderQty) — simple & robust."""
        return self._signed_request(
            "POST", "/order", {"symbol": symbol, "side": "BUY", "type": "MARKET", "quoteOrderQty": _fmt(quote_qty)}
        )

    def market_sell(self, symbol: str, base_qty: float) -> dict:
        return self._signed_request(
            "POST", "/order", {"symbol": symbol, "side": "SELL", "type": "MARKET", "quantity": _fmt(base_qty)}
        )

    def open_orders(self, symbol: Optional[str] = None) -> list:
        p = {"symbol": symbol} if symbol else {}
        return self._signed_request("GET", "/openOrders", p)

    def cancel_all(self, symbol: str) -> dict:
        return self._signed_request("DELETE", "/openOrders", {"symbol": symbol})

    def my_trades(self, symbol: str, limit: int = 50) -> list:
        return self._signed_request("GET", "/myTrades", {"symbol": symbol, "limit": limit})


def _fmt(x: float) -> str:
    """Trim to 8 decimals, no trailing zeros (Binance max precision)."""
    s = f"{x:.8f}".rstrip("0").rstrip(".")
    return s if s else "0"


class PaperClient:
    """
    Simulated wallet backed by real market data.

    state = {
        "cash": 10000.0,                # USDT
        "positions": {"BTCUSDT": {"qty": 0.01, "cost": 38000.0, "ts": ...}},
        "trade_seq": 123,
        "init_balance": 10000.0,
    }
    """

    def __init__(self, data_client: BinanceClient):
        self.data = data_client  # any BinanceClient for prices/klines

    # ---- market data passthrough ----
    def klines(self, symbol: str, interval: str, limit: int = 200) -> List[dict]:
        return self.data.klines(symbol, interval, limit)

    def ticker_price(self, symbol: str) -> float:
        return self.data.ticker_price(symbol)

    # ---- wallet ----
    def balance(self, state: dict) -> dict:
        return {
            "cash": state["cash"],
            "positions": dict(state.get("positions", {})),
        }

    def market_buy(self, symbol: str, quote_qty: float, price: float, state: dict) -> dict:
        if quote_qty <= 0:
            raise ExchangeError("quote qty must be > 0")
        if quote_qty > state["cash"]:
            raise ExchangeError(
                f"insufficient cash: need {quote_qty:.2f} have {state['cash']:.2f}"
            )
        fee = quote_qty * FEE_RATE
        qty = (quote_qty - fee) / price
        pos = state["positions"].get(symbol)
        if pos:
            total_cost = pos["cost"] * pos["qty"] + quote_qty
            pos["qty"] += qty
            pos["cost"] = total_cost / pos["qty"]
        else:
            state["positions"][symbol] = {"qty": qty, "cost": price, "ts": _utc_ms()}
        state["cash"] -= quote_qty
        return {
            "symbol": symbol,
            "orderId": state.get("trade_seq", 0) + 1,
            "executedQty": qty,
            "cummulativeQuoteQty": quote_qty,
            "price": price,
            "fee": fee,
        }

    def market_sell(self, symbol: str, base_qty: float, price: float, state: dict) -> dict:
        pos = state["positions"].get(symbol)
        if not pos or pos["qty"] < base_qty - 1e-12:
            raise ExchangeError(f"insufficient {symbol} balance")
        gross = base_qty * price
        fee = gross * FEE_RATE
        proceeds = gross - fee
        pos["qty"] -= base_qty
        if pos["qty"] <= 1e-12:
            del state["positions"][symbol]
        state["cash"] += proceeds
        return {
            "symbol": symbol,
            "orderId": state.get("trade_seq", 0) + 1,
        }
