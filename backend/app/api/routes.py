"""REST API — dashboard endpoints."""
from __future__ import annotations

import re
from typing import Optional

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.config import DEFAULT_CONFIG, get_settings
from app.database import get_db
from app.engine.engine import engine
from app.exchange.binance import MAINNET_BASE, TESTNET_BASE, BinanceClient
from app.models import BotConfig, BotState, EquityPoint, LogEntry, OrderLog, Trade
from app.strategy.strategies import STRATEGIES

router = APIRouter()

PUBLIC_FIELDS = {
    "trading_mode", "paper_data_source", "start_balance", "trading_symbols",
    "timeframe", "strategy", "stake_mode", "stake_amount", "stake_percent",
    "stop_loss_pct", "take_profit_pct", "max_open_trades", "poll_interval",
    "bot_running", "trailing_stop", "trailing_stop_pct", "grid_levels",
}


# ------------------------------------------------------------------ helpers
def _cfg_row(db: Session) -> BotConfig:
    row = db.execute(select(BotConfig)).scalar_one_or_none()
    if row is None:
        row = BotConfig(config=dict(DEFAULT_CONFIG), credentials={})
        db.add(row)
        db.commit()
    return row


def _public_config(row: BotConfig) -> dict:
    return {k: v for k, v in (row.config or {}).items() if k != "reset_requested"}


def _bot_state(db: Session) -> dict:
    row = db.execute(select(BotState)).scalar_one_or_none()
    if row and row.state:
        return dict(row.state)
    cfg = _public_config(_cfg_row(db))
    return {"cash": float(cfg.get("start_balance", 10000)), "positions": {}, "init_balance": float(cfg.get("start_balance", 10000))}


def _data_client(db: Session) -> BinanceClient:
    cfg = _public_config(_cfg_row(db))
    source = cfg.get("paper_data_source", "testnet")
    base = MAINNET_BASE if source == "mainnet" else TESTNET_BASE
    return BinanceClient(base_url=base)


# ------------------------------------------------------------------ schemas
class ConfigUpdate(BaseModel):
    trading_mode: Optional[str] = None
    paper_data_source: Optional[str] = None
    start_balance: Optional[float] = None
    trading_symbols: Optional[str] = None
    timeframe: Optional[str] = None
    strategy: Optional[str] = None
    stake_mode: Optional[str] = None
    stake_amount: Optional[float] = None
    stake_percent: Optional[float] = None
    stop_loss_pct: Optional[float] = None
    take_profit_pct: Optional[float] = None
    max_open_trades: Optional[int] = None
    poll_interval: Optional[int] = None
    bot_running: Optional[bool] = None
    trailing_stop: Optional[bool] = None
    trailing_stop_pct: Optional[float] = None
    grid_levels: Optional[int] = None


class ManualBuy(BaseModel):
    symbol: str
    stake: float


class CredentialsIn(BaseModel):
    binance_api_key: str = ""
    binance_api_secret: str = ""


# ------------------------------------------------------------------ status/overview
@router.get("/status")
def status(db: Session = Depends(get_db)):
    cfg = _public_config(_cfg_row(db))
    state = _bot_state(db)
    last = dict(engine._last_cycle or {})
    prices = last.get("prices", {})

    open_trades = list(db.execute(select(Trade).where(Trade.status == "open")).scalars())
    closed = list(db.execute(select(Trade).where(Trade.status == "closed")).scalars())
    wins = [t for t in closed if t.pnl > 0]
    losses = [t for t in closed if t.pnl <= 0]

    pos_val = 0.0
    for sym, pos in state.get("positions", {}).items():
        p = prices.get(sym, pos.get("cost", 0))
        pos_val += pos["qty"] * p
    cash = state.get("cash", 0.0)
    equity = cash + pos_val
    start_bal = float(state.get("init_balance", cfg.get("start_balance", 10000)))

    avg_win = sum(t.pnl for t in wins) / len(wins) if wins else 0.0
    avg_loss = sum(t.pnl for t in losses) / len(losses) if losses else 0.0

    return {
        "running": bool(cfg.get("bot_running", True)),
        "mode": cfg.get("trading_mode", "paper"),
        "last_cycle": last,
        "equity": round(equity, 2),
        "cash": round(cash, 2),
        "positions_value": round(pos_val, 2),
        "start_balance": start_bal,
        "profit": round(equity - start_bal, 2),
        "profit_pct": round((equity - start_bal) / start_bal * 100, 2) if start_bal else 0,
        "open_trades": [t.to_dict() for t in open_trades],
        "stats": {
            "total_trades": len(closed) + len(open_trades),
            "closed_trades": len(closed),
            "wins": len(wins),
            "losses": len(losses),
            "win_rate": round(len(wins) / len(closed) * 100, 2) if closed else 0.0,
            "total_pnl": round(sum(t.pnl for t in closed), 2),
            "avg_win": round(avg_win, 2),
            "avg_loss": round(avg_loss, 2),
            "max_drawdown": _max_drawdown(db),
            "best_trade": round(max((t.pnl for t in closed), default=0.0), 2),
            "worst_trade": round(min((t.pnl for t in closed), default=0.0), 2),
        },
    }


def _max_drawdown(db: Session) -> float:
    pts = list(db.execute(select(EquityPoint).order_by(EquityPoint.id)).scalars())
    if len(pts) < 2:
        return 0.0
    peak, mdd = pts[0].equity, 0.0
    for p in pts:
        peak = max(peak, p.equity)
        if peak:
            mdd = max(mdd, (peak - p.equity) / peak * 100)
    return round(mdd, 2)


# ------------------------------------------------------------------ market data
@router.get("/candles")
def candles(
    symbol: str = "BTCUSDT",
    interval: str = "5m",
    limit: int = Query(200, le=500),
    db: Session = Depends(get_db),
):
    client = _data_client(db)
    data = client.klines(symbol.upper(), interval, limit)
    return {"symbol": symbol.upper(), "interval": interval, "candles": data}


@router.get("/symbols")
def symbols(db: Session = Depends(get_db)):
    client = _data_client(db)
    info = client.exchange_info()
    return {
        "symbols": [
            s["symbol"] for s in info if s.get("quote") == "USDT" and s.get("status") == "TRADING"
        ]
    }


@router.get("/ticker")
def ticker(symbols: str = "BTCUSDT,ETHUSDT", db: Session = Depends(get_db)):
    client = _data_client(db)
    out = {}
    for s in symbols.split(","):
        s = s.strip().upper()
        if s:
            try:
                out[s] = client.ticker_price(s)
            except Exception:
                pass
    return {"prices": out}


# ------------------------------------------------------------------ data endpoints
@router.get("/trades")
def trades(
    status: Optional[str] = None,
    limit: int = Query(100, le=500),
    db: Session = Depends(get_db),
):
    q = select(Trade).order_by(Trade.id.desc())
    if status:
        q = q.where(Trade.status == status)
    rows = list(db.execute(q.limit(limit)).scalars())
    return {"trades": [t.to_dict() for t in rows]}


@router.get("/activity")
def activity(limit: int = Query(100, le=500), db: Session = Depends(get_db)):
    rows = list(db.execute(select(OrderLog).order_by(OrderLog.id.desc()).limit(limit)).scalars())
    return {"activity": [r.to_dict() for r in rows]}


@router.get("/logs")
def logs(
    level: Optional[str] = None,
    limit: int = Query(100, le=500),
    db: Session = Depends(get_db),
):
    q = select(LogEntry).order_by(LogEntry.id.desc())
    if level:
        q = q.where(LogEntry.level == level.upper())
    rows = list(db.execute(q.limit(limit)).scalars())
    return {"logs": [r.to_dict() for r in rows]}


@router.get("/equity")
def equity(limit: int = Query(300, le=2000), db: Session = Depends(get_db)):
    rows = list(db.execute(select(EquityPoint).order_by(EquityPoint.id.desc()).limit(limit)).scalars())
    rows.reverse()
    return {"points": [r.to_dict() for r in rows]}


# ------------------------------------------------------------------ config
@router.get("/config")
def get_config(db: Session = Depends(get_db)):
    row = _cfg_row(db)
    out = _public_config(row)
    out["strategies_available"] = [{"id": k, "label": v.label} for k, v in STRATEGIES.items()]
    out["has_credentials"] = bool((row.credentials or {}).get("binance_api_key"))
    out["timeframes"] = ["1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "1d"]
    return out


@router.post("/config")
def update_config(payload: ConfigUpdate, db: Session = Depends(get_db)):
    row = _cfg_row(db)
    cfg = dict(row.config or {})
    data = payload.model_dump(exclude_unset=True, exclude_none=True)
    changed = {}
    for k, v in data.items():
        if k in PUBLIC_FIELDS:
            cfg[k] = v
            changed[k] = v
    row.config = cfg
    db.commit()
    engine.kick()
    return {"ok": True, "updated": changed, "config": _public_config(row)}


@router.post("/credentials")
def update_credentials(payload: CredentialsIn, db: Session = Depends(get_db)):
    key = (payload.binance_api_key or "").strip()
    secret = (payload.binance_api_secret or "").strip()
    if key and not re.match(r"^[A-Za-z0-9_\-]{16,}$", key):
        raise HTTPException(400, "API key format looks wrong")
    row = _cfg_row(db)
    row.credentials = {"binance_api_key": key, "binance_api_secret": secret}
    db.commit()
    engine.kick()
    return {"ok": True, "has_credentials": bool(key)}


@router.post("/credentials/clear")
def clear_credentials(db: Session = Depends(get_db)):
    row = _cfg_row(db)
    row.credentials = {}
    db.commit()
    return {"ok": True}


# ------------------------------------------------------------------ manual trading
@router.post("/buy")
def manual_buy(payload: ManualBuy):
    res = engine.manual_buy(payload.symbol, payload.stake)
    if not res.get("ok"):
        raise HTTPException(400, res.get("error", "buy failed"))
    return res


@router.post("/sell/{trade_id}")
def manual_sell(trade_id: int):
    res = engine.manual_sell(trade_id)
    if not res.get("ok"):
        raise HTTPException(400, res.get("error", "sell failed"))
    return res


@router.post("/reset")
def reset_paper():
    return engine.reset()


# ------------------------------------------------------------------ account
@router.get("/account")
def account(db: Session = Depends(get_db)):
    """Live testnet account balances (live mode with credentials only)."""
    cfg = _public_config(_cfg_row(db))
    row = _cfg_row(db)
    cred = row.credentials or {}
    if cfg.get("trading_mode") != "live" or not cred.get("binance_api_key"):
        return {"live": False, "balances": []}
    client = BinanceClient(cred["binance_api_key"], cred["binance_api_secret"])
    try:
        acc = client.account()
        return {"live": True, **acc}
    except Exception as e:
        return {"live": False, "error": str(e), "balances": []}
