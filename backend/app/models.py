"""Database models: trades, orders, equity snapshots, logs, bot config."""
from __future__ import annotations

from datetime import datetime, timezone

from sqlalchemy import Boolean, DateTime, Float, Integer, String, Text, JSON
from sqlalchemy.orm import Mapped, mapped_column

from app.database import Base


def utcnow() -> datetime:
    return datetime.now(timezone.utc)


class Trade(Base):
    __tablename__ = "trades"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    symbol: Mapped[str] = mapped_column(String(20), index=True)
    side: Mapped[str] = mapped_column(String(10))  # buy | sell
    status: Mapped[str] = mapped_column(String(20), default="open", index=True)  # open | closed
    mode: Mapped[str] = mapped_column(String(10), default="paper")  # paper | live

    qty: Mapped[float] = mapped_column(Float, default=0.0)
    entry_price: Mapped[float] = mapped_column(Float, default=0.0)
    exit_price: Mapped[float | None] = mapped_column(Float, nullable=True)
    stop_loss: Mapped[float] = mapped_column(Float, default=0.0)
    take_profit: Mapped[float] = mapped_column(Float, default=0.0)

    stake: Mapped[float] = mapped_column(Float, default=0.0)  # quote spent on entry
    pnl: Mapped[float] = mapped_column(Float, default=0.0)
    pnl_pct: Mapped[float] = mapped_column(Float, default=0.0)
    fee: Mapped[float] = mapped_column(Float, default=0.0)

    strategy: Mapped[str] = mapped_column(String(50), default="")
    signal_reason: Mapped[str] = mapped_column(Text, default="")
    exit_reason: Mapped[str] = mapped_column(String(30), default="")  # signal | stop_loss | take_profit | manual

    opened_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
    closed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    meta: Mapped[dict] = mapped_column(JSON, default=dict)  # grid_count, initial_stake, trail_activated…

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "symbol": self.symbol,
            "side": self.side,
            "status": self.status,
            "mode": self.mode,
            "qty": self.qty,
            "entry_price": self.entry_price,
            "exit_price": self.exit_price,
            "stop_loss": self.stop_loss,
            "take_profit": self.take_profit,
            "stake": self.stake,
            "pnl": self.pnl,
            "pnl_pct": self.pnl_pct,
            "fee": self.fee,
            "strategy": self.strategy,
            "signal_reason": self.signal_reason,
            "exit_reason": self.exit_reason,
            "opened_at": self.opened_at.isoformat() if self.opened_at else None,
            "closed_at": self.closed_at.isoformat() if self.closed_at else None,
            "meta": self.meta or {},
        }


class OrderLog(Base):
    """Every exchange interaction (paper or live) for the activity feed."""
    __tablename__ = "order_logs"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    trade_id: Mapped[int | None] = mapped_column(Integer, nullable=True)
    symbol: Mapped[str] = mapped_column(String(20))
    action: Mapped[str] = mapped_column(String(20))  # buy | sell | cancel | info | error
    mode: Mapped[str] = mapped_column(String(10), default="paper")
    qty: Mapped[float | None] = mapped_column(Float, nullable=True)
    price: Mapped[float | None] = mapped_column(Float, nullable=True)
    status: Mapped[str] = mapped_column(String(20), default="ok")  # ok | error
    detail: Mapped[str] = mapped_column(Text, default="")
    exchange_order_id: Mapped[str | None] = mapped_column(String(64), nullable=True)
    raw: Mapped[str | None] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow, index=True)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "trade_id": self.trade_id,
            "symbol": self.symbol,
            "action": self.action,
            "mode": self.mode,
            "qty": self.qty,
            "price": self.price,
            "status": self.status,
            "detail": self.detail,
            "exchange_order_id": self.exchange_order_id,
            "raw": self.raw,
            "created_at": self.created_at.isoformat() if self.created_at else None,
        }


class EquityPoint(Base):
    """Equity curve snapshots (paper wallet total value over time)."""
    __tablename__ = "equity_points"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    ts: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow, index=True)
    equity: Mapped[float] = mapped_column(Float)
    cash: Mapped[float] = mapped_column(Float)
    positions_value: Mapped[float] = mapped_column(Float)
    mode: Mapped[str] = mapped_column(String(10), default="paper")

    def to_dict(self) -> dict:
        return {
            "ts": self.ts.isoformat() if self.ts else None,
            "equity": self.equity,
            "cash": self.cash,
            "positions_value": self.positions_value,
        }


class LogEntry(Base):
    __tablename__ = "logs"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    level: Mapped[str] = mapped_column(String(10), default="INFO")  # INFO | WARN | ERROR | DEBUG
    module: Mapped[str] = mapped_column(String(30), default="engine")
    message: Mapped[str] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow, index=True)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "level": self.level,
            "module": self.module,
            "message": self.message,
            "created_at": self.created_at.isoformat() if self.created_at else None,
        }


class BotConfig(Base):
    """Single-row runtime config, editable from the dashboard."""
    __tablename__ = "bot_config"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    config: Mapped[dict] = mapped_column(JSON, default=dict)
    credentials: Mapped[dict] = mapped_column(JSON, default=dict)
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)


class BotState(Base):
    """Single-row paper wallet state (cash balance etc.)."""
    __tablename__ = "bot_state"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    state: Mapped[dict] = mapped_column(JSON, default=dict)
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=utcnow)
