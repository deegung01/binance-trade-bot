"""
Trading engine — the bot's main loop.

Every poll interval:
  1. Load config + state from DB
  2. For each symbol: fetch candles -> indicators -> entry/exit signals
  3. Manage open trades (signal exit / stop-loss / take-profit)
  4. Open new trades when entry signal + risk checks pass
  5. Snapshot equity, log everything
"""
from __future__ import annotations

import json
import threading
import time
import traceback
from datetime import datetime, timezone
from typing import Dict, List, Optional

from sqlalchemy import delete, select
from sqlalchemy.orm import Session

from app.config import DEFAULT_CONFIG, get_settings, seed_credentials
from app.database import SessionLocal
from app.exchange.binance import (
    MAINNET_BASE,
    TESTNET_BASE,
    BinanceClient,
    ExchangeError,
    PaperClient,
)
from app.models import (
    BotConfig,
    BotState,
    EquityPoint,
    LogEntry,
    OrderLog,
    Trade,
    utcnow,
)
from app.strategy.strategies import get_strategy


def utc_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


class TradingEngine:
    def __init__(self):
        self._thread: Optional[threading.Thread] = None
        self._stop = threading.Event()
        self._wake = threading.Event()
        self._lock = threading.Lock()
        self._last_cycle: Dict[str, any] = {}

    # ------------------------------------------------------------------ boot
    def start(self):
        if self._thread and self._thread.is_alive():
            return
        self._stop.clear()
        self._thread = threading.Thread(target=self._run, name="engine", daemon=True)
        self._thread.start()

    def stop(self):
        self._stop.set()
        self._wake.set()

    def kick(self):
        """Wake the loop immediately (after config changes)."""
        self._wake.set()

    # ------------------------------------------------------------------ loop
    def _run(self):
        # wait for tables
        time.sleep(0.5)
        while not self._stop.is_set():
            try:
                with self._lock:
                    self._cycle()
            except Exception as e:
                try:
                    self._log("ERROR", "loop", f"cycle failed: {e}\n{traceback.format_exc()[-800:]}")
                except Exception:
                    print("engine error:", e, flush=True)
            interval = max(3, self._poll_interval())
            self._wake.wait(timeout=interval)
            self._wake.clear()

    def _poll_interval(self) -> int:
        try:
            with SessionLocal() as db:
                cfg = self._load_config(db)
                return int(cfg.get("poll_interval", 15))
        except Exception:
            return 15

    # ------------------------------------------------------------------ config/state
    def _load_config(self, db: Session) -> dict:
        row = db.execute(select(BotConfig)).scalar_one_or_none()
        if row is None:
            row = BotConfig(config=dict(DEFAULT_CONFIG), credentials=seed_credentials())
            db.add(row)
            db.commit()
        cfg = dict(DEFAULT_CONFIG)
        cfg.update(row.config or {})
        return cfg

    def _load_state(self, db: Session, cfg: dict) -> dict:
        row = db.execute(select(BotState)).scalar_one_or_none()
        cfg_row = db.execute(select(BotConfig)).scalar_one_or_none()
        needs_reset = bool(cfg_row and cfg_row.config and cfg_row.config.get("reset_requested"))
        if row is None or not row.state or needs_reset:
            state = {
                "cash": float(cfg.get("start_balance", 10000)),
                "positions": {},
                "trade_seq": 0,
                "init_balance": float(cfg.get("start_balance", 10000)),
                "started_at": utcnow().isoformat(),
            }
            if row is None:
                row = BotState()
                row.state = state
                db.add(row)
            else:
                row.state = state  # reassign so SQLAlchemy detects the change
                row.updated_at = utcnow()
            # consume reset flag — MUST reassign (JSON columns don't track mutations)
            if cfg_row is not None and needs_reset:
                new_cfg = dict(cfg_row.config or {})
                new_cfg["reset_requested"] = False
                cfg_row.config = new_cfg
            db.commit()
            if needs_reset:
                # wipe history on explicit reset
                for table in (Trade, OrderLog, EquityPoint, LogEntry):
                    db.execute(delete(table))
                db.commit()
                return state
            return state
        return dict(row.state)

    def _save_state(self, db: Session, state: dict):
        row = db.execute(select(BotState)).scalar_one_or_none()
        if row is None:
            row = BotState(state=state)
            db.add(row)
        else:
            row.state = state
            row.updated_at = utcnow()
        db.commit()

    def _clients(self, db: Session, cfg: dict):
        """Build (mode, client) according to config."""
        cred_row = db.execute(select(BotConfig)).scalar_one_or_none()
        creds = (cred_row.credentials if cred_row else None) or {}
        api_key = creds.get("binance_api_key", "")
        api_secret = creds.get("binance_api_secret", "")
        if not api_key:
            s = get_settings()
            api_key, api_secret = s.binance_api_key, s.binance_api_secret

        data_source = cfg.get("paper_data_source", "testnet")
        data_base = MAINNET_BASE if data_source == "mainnet" else TESTNET_BASE
        data_client = BinanceClient(api_key, api_secret, base_url=data_base)

        mode = cfg.get("trading_mode", "paper")
        if mode == "live" and api_key and api_secret:
            live = BinanceClient(api_key, api_secret, base_url=TESTNET_BASE)
            return "live", live
        # paper
        return "paper", PaperClient(data_client)

    def _log(self, level: str, module: str, message: str):
        try:
            with SessionLocal() as db:
                db.add(LogEntry(level=level, module=module, message=message[:2000]))
                db.commit()
        except Exception:
            pass

    def _order_log(self, db: Session, **kw):
        db.add(OrderLog(**kw))
        db.commit()

    # ------------------------------------------------------------------ cycle
    def _cycle(self):
        db = SessionLocal()
        try:
            cfg = self._load_config(db)
            running = bool(cfg.get("bot_running", True))
            mode, client = self._clients(db, cfg)
            state = self._load_state(db, cfg)
            symbols = [s.strip().upper() for s in str(cfg.get("trading_symbols", "")).split(",") if s.strip()]
            timeframe = cfg.get("timeframe", "5m")
            strategy_name = cfg.get("strategy", "sma_cross")

            if not running:
                self._last_cycle = {"running": False, "ts": utc_iso()}
                return

            strategy = get_strategy(strategy_name)
            prices: Dict[str, float] = {}

            # 1) candles + prices for every symbol
            candles_map: Dict[str, list] = {}
            for sym in symbols:
                try:
                    candles_map[sym] = client.klines(sym, timeframe, 200)
                    prices[sym] = candles_map[sym][-1]["close"]
                except ExchangeError as e:
                    self._log("WARN", "data", f"klines {sym} failed: {e}")

            # 2) manage open trades
            open_trades = list(db.execute(select(Trade).where(Trade.status == "open")).scalars())
            for trade in open_trades:
                if trade.symbol not in prices:
                    continue
                price = prices[trade.symbol]
                self._manage_trade(db, cfg, mode, client, state, trade, price, candles_map.get(trade.symbol))

            # 3) entry signals
            open_count = db.execute(
                select(Trade).where(Trade.status == "open")
            ).scalars().all()
            open_symbols = {t.symbol for t in open_count}
            max_open = int(cfg.get("max_open_trades", 3))

            if len(open_count) < max_open:
                for sym in symbols:
                    if sym in open_symbols:
                        continue  # one trade per symbol
                    candles = candles_map.get(sym)
                    if not candles:
                        continue
                    sig = strategy.entry_signal(candles)
                    if not sig:
                        continue
                    side, reason = sig
                    price = prices[sym]
                    stake = self._stake(cfg, state)
                    if stake <= 0:
                        self._log("WARN", "risk", "stake is 0, skipping entry")
                        break
                    self._open_trade(db, cfg, mode, client, state, sym, stake, price, strategy_name, reason)
                    if len(open_count) + 1 >= max_open:
                        break

            # 4) equity snapshot
            state["trade_seq"] = state.get("trade_seq", 0) + 1
            self._save_state(db, state)
            equity = self._equity(state, prices)
            db.add(
                EquityPoint(
                    equity=equity["total"],
                    cash=equity["cash"],
                    positions_value=equity["positions_value"],
                    mode=mode,
                )
            )
            db.commit()

            self._last_cycle = {
                "running": True,
                "ts": utc_iso(),
                "mode": mode,
                "symbols": symbols,
                "prices": {k: round(v, 6) for k, v in prices.items()},
                "strategy": strategy_name,
                "timeframe": timeframe,
                "open_trades": len(open_count),
            }
        finally:
            db.close()

    # ------------------------------------------------------------------ trade ops
    def _stake(self, cfg: dict, state: dict) -> float:
        if cfg.get("stake_mode") == "percent":
            pct = float(cfg.get("stake_percent", 10)) / 100.0
            return round(state["cash"] * pct, 2)
        return round(float(cfg.get("stake_amount", 100)), 2)

    def _equity(self, state: dict, prices: Dict[str, float]) -> dict:
        cash = state.get("cash", 0.0)
        pos_val = 0.0
        for sym, pos in state.get("positions", {}).items():
            price = prices.get(sym)
            if price is None:
                price = pos.get("cost", 0.0)  # fallback: entry cost
            pos_val += pos["qty"] * price
        return {"cash": cash, "positions_value": pos_val, "total": cash + pos_val}

    def _open_trade(
        self, db, cfg, mode, client, state, symbol, stake, price, strategy_name, reason
    ):
        try:
            if mode == "paper":
                res = client.market_buy(symbol, stake, price, state)
                qty, fee = res["executedQty"], res["fee"]
                oid = None
            else:
                res = client.market_buy(symbol, stake)
                qty = float(res.get("executedQty", 0))
                cum = float(res.get("cummulativeQuoteQty", stake))
                price = cum / qty if qty else price
                fee = 0.0
                oid = str(res.get("orderId", ""))
            sl_pct = float(cfg.get("stop_loss_pct", 2)) / 100
            tp_pct = float(cfg.get("take_profit_pct", 4)) / 100
            trade = Trade(
                symbol=symbol,
                side="buy",
                status="open",
                mode=mode,
                qty=qty,
                entry_price=price,
                stop_loss=price * (1 - sl_pct),
                take_profit=price * (1 + tp_pct),
                stake=stake,
                fee=fee,
                strategy=strategy_name,
                signal_reason=reason,
                meta={
                    "grid_count": 0,
                    "initial_stake": stake,
                    "last_add_price": price,
                    "trail_activated": False,
                    "trail_high": price,
                },
            )
            db.add(trade)
            db.commit()
            self._order_log(
                db,
                trade_id=trade.id,
                symbol=symbol,
                action="buy",
                mode=mode,
                qty=qty,
                price=price,
                status="ok",
                detail=f"entry: {reason}",
                exchange_order_id=oid,
            )
            self._log("INFO", "trade", f"BUY {symbol} qty={qty:.6f} @ {price} ({reason})")
        except ExchangeError as e:
            self._order_log(
                db, symbol=symbol, action="error", mode=mode, status="error", detail=f"buy failed: {e}"
            )
            self._log("ERROR", "trade", f"BUY {symbol} failed: {e}")
            db.rollback()

    def _manage_trade(self, db, cfg, mode, client, state, trade: Trade, price: float, candles):
        meta = dict(trade.meta or {})

        # ---- 1) TRAILING STOP (chỉ nâng SL lên, không bao giờ hạ xuống) ----
        trailing_on = bool(cfg.get("trailing_stop", False))
        trail_pct = float(cfg.get("trailing_stop_pct", 1.0)) / 100.0
        if trailing_on and trade.status == "open":
            prev_high = float(meta.get("trail_high", trade.entry_price))
            if price > prev_high:
                meta["trail_high"] = price
                prev_high = price
            # kích hoạt khi lãi ≥ trail_pct*2 (đủ đệm), sau đó SL bám theo high
            gain_pct = (price - trade.entry_price) / trade.entry_price
            if not meta.get("trail_activated") and gain_pct >= trail_pct * 2:
                meta["trail_activated"] = True
            if meta.get("trail_activated"):
                new_sl = prev_high * (1 - trail_pct)
                if new_sl > trade.stop_loss:
                    trade.stop_loss = new_sl
                    trade.meta = meta  # reassign để SQLAlchemy nhận diện thay đổi
                    db.commit()
                    self._log(
                        "INFO",
                        "trail",
                        f"TRAIL {trade.symbol}: SL nâng lên {new_sl:.4f} (high {prev_high:.4f})",
                    )
                else:
                    trade.meta = meta
                    db.commit()
            else:
                trade.meta = meta
                db.commit()

        # ---- 2) EXIT CHECKS ----
        exit_reason = None
        if price <= trade.stop_loss:
            exit_reason = "stop_loss" if not meta.get("trail_activated") else "trailing_stop"
        elif price >= trade.take_profit:
            exit_reason = "take_profit"
        else:
            try:
                strategy = get_strategy(trade.strategy or cfg.get("strategy", "sma_cross"))
                if candles and strategy.exit_signal(candles, trade):
                    exit_reason = "signal"
            except Exception:
                pass

        if exit_reason:
            self._close_trade(db, mode, client, state, trade, price, exit_reason)
            return

        # ---- 3) GRID DCA ADD (adaptive grid) ----
        try:
            strategy = get_strategy(trade.strategy or cfg.get("strategy", "sma_cross"))
            adjust = getattr(strategy, "adjust_signal", None)
            if adjust and candles:
                extra_usdt = adjust(candles, trade, cfg)
                if extra_usdt and extra_usdt > 0:
                    self._grid_add(db, mode, client, state, trade, extra_usdt, price, strategy)
        except Exception:
            pass

    def _grid_add(self, db, mode, client, state, trade: Trade, extra_usdt: float, price: float, strategy):
        """DCA add một grid level: mua thêm, rebase avg cost + SL/TP theo cost mới."""
        try:
            meta = dict(trade.meta or {})
            if mode == "paper":
                if extra_usdt > state["cash"]:
                    self._log("WARN", "grid", f"skip grid add {trade.symbol}: not enough cash")
                    return
                res = client.market_buy(trade.symbol, extra_usdt, price, state)
                add_qty, add_fee = res["executedQty"], res["fee"]
            else:
                res = client.market_buy(trade.symbol, extra_usdt)
                add_qty = float(res.get("executedQty", 0))
                cum = float(res.get("cummulativeQuoteQty", extra_usdt))
                price = cum / add_qty if add_qty else price
                add_fee = 0.0

            # rebase weighted avg entry
            total_cost = trade.entry_price * trade.qty + extra_usdt
            new_qty = trade.qty + add_qty
            new_avg = total_cost / new_qty if new_qty else trade.entry_price

            old_sl_gap = (trade.entry_price - trade.stop_loss) / trade.entry_price if trade.entry_price else 0
            old_tp_gap = (trade.take_profit - trade.entry_price) / trade.entry_price if trade.entry_price else 0

            trade.qty = new_qty
            trade.entry_price = new_avg
            trade.stake = trade.stake + extra_usdt
            trade.fee += add_fee
            trade.stop_loss = new_avg * (1 - old_sl_gap)
            trade.take_profit = new_avg * (1 + old_tp_gap)
            meta["grid_count"] = int(meta.get("grid_count", 0)) + 1
            meta["last_add_price"] = price
            trade.meta = meta
            db.commit()
            self._order_log(
                db,
                trade_id=trade.id,
                symbol=trade.symbol,
                action="grid_add",
                mode=mode,
                qty=add_qty,
                price=price,
                status="ok",
                detail=f"DCA add level {meta['grid_count']}: +{extra_usdt:.0f} USDT, new avg {new_avg:.4f}",
            )
            self._log(
                "INFO",
                "grid",
                f"GRID ADD {trade.symbol} +{add_qty:.6f} @ {price} → avg {new_avg:.4f} (level {meta['grid_count']})",
            )
        except ExchangeError as e:
            self._order_log(
                db, trade_id=trade.id, symbol=trade.symbol, action="error", mode=mode,
                status="error", detail=f"grid add failed: {e}",
            )
            self._log("ERROR", "grid", f"GRID ADD {trade.symbol} failed: {e}")
            db.rollback()

    def _close_trade(self, db, mode, client, state, trade: Trade, price: float, reason: str):
        try:
            if mode == "paper":
                res = client.market_sell(trade.symbol, trade.qty, price, state)
                fee = res.get("fee", 0.0)
            else:
                res = client.market_sell(trade.symbol, trade.qty)
                qty = float(res.get("executedQty", trade.qty))
                cum = float(res.get("cummulativeQuoteQty", 0)) or price * qty
                price = cum / qty if qty else price
                fee = 0.0
            gross = trade.qty * price
            pnl = gross - trade.stake
            pnl_pct = (pnl / trade.stake * 100) if trade.stake else 0.0
            trade.status = "closed"
            trade.exit_price = price
            trade.pnl = pnl
            trade.pnl_pct = pnl_pct
            trade.fee += fee
            trade.exit_reason = reason
            trade.closed_at = utcnow()
            db.commit()
            self._order_log(
                db,
                trade_id=trade.id,
                symbol=trade.symbol,
                action="sell",
                mode=mode,
                qty=trade.qty,
                price=price,
                status="ok",
                detail=f"exit ({reason}) pnl={pnl:+.2f}",
            )
            self._log("INFO", "trade", f"SELL {trade.symbol} @ {price} reason={reason} pnl={pnl:+.2f}")
        except ExchangeError as e:
            self._order_log(
                db, trade_id=trade.id, symbol=trade.symbol, action="error", mode=mode, status="error",
                detail=f"sell failed: {e}",
            )
            self._log("ERROR", "trade", f"SELL {trade.symbol} failed: {e}")
            db.rollback()

    # ------------------------------------------------------------------ manual ops (called by API)
    def manual_buy(self, symbol: str, stake: float) -> dict:
        with self._lock:
            db = SessionLocal()
            try:
                cfg = self._load_config(db)
                mode, client = self._clients(db, cfg)
                state = self._load_state(db, cfg)
                price = client.ticker_price(symbol)
                stake = round(float(stake), 2)
                self._open_trade(db, cfg, mode, client, state, symbol.upper(), stake, price, "manual", "manual order")
                self._save_state(db, state)
                return {"ok": True, "price": price}
            finally:
                db.close()

    def manual_sell(self, trade_id: int) -> dict:
        with self._lock:
            db = SessionLocal()
            try:
                cfg = self._load_config(db)
                mode, client = self._clients(db, cfg)
                state = self._load_state(db, cfg)
                trade = db.get(Trade, trade_id)
                if not trade or trade.status != "open":
                    return {"ok": False, "error": "trade not found or already closed"}
                price = client.ticker_price(trade.symbol)
                self._close_trade(db, mode, client, state, trade, price, "manual")
                self._save_state(db, state)
                return {"ok": True}
            finally:
                db.close()

    def reset(self) -> dict:
        with self._lock:
            db = SessionLocal()
            try:
                row = db.execute(select(BotConfig)).scalar_one()
                cfg = dict(row.config or {})
                cfg["reset_requested"] = True
                row.config = cfg
                db.commit()
                self._load_state(db, cfg)  # rebuilds + wipes tables
                self._log("INFO", "engine", "paper wallet reset")
                return {"ok": True}
            finally:
                db.close()


engine = TradingEngine()
