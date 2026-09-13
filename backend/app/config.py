"""Application configuration loaded from environment / .env file."""
from __future__ import annotations

import json
import os
from pathlib import Path
from typing import List, Optional

from pydantic_settings import BaseSettings

BASE_DIR = Path(__file__).resolve().parent.parent
DATA_DIR = BASE_DIR / "data"
DATA_DIR.mkdir(parents=True, exist_ok=True)


class Settings(BaseSettings):
    # Binance testnet credentials
    binance_api_key: str = ""
    binance_api_secret: str = ""

    # paper | live
    trading_mode: str = "paper"
    paper_data_source: str = "testnet"

    # Bot defaults (seeded into DB config on first start)
    start_balance: float = 10000.0
    trading_symbols: str = "BTCUSDT,ETHUSDT,SOLUSDT,BNBUSDT"
    timeframe: str = "5m"
    strategy: str = "sma_cross"
    stake_mode: str = "fixed"  # fixed | percent
    stake_amount: float = 100.0
    stake_percent: float = 10.0
    stop_loss_pct: float = 2.0
    take_profit_pct: float = 4.0
    max_open_trades: int = 3
    poll_interval: int = 15

    # Security
    bot_api_token: str = ""

    # Database
    database_url: str = ""

    # CORS
    allowed_origins: str = "*"

    class Config:
        env_file = str(BASE_DIR / ".env")
        env_file_encoding = "utf-8"

    @property
    def symbols(self) -> List[str]:
        return [s.strip().upper() for s in self.trading_symbols.split(",") if s.strip()]

    @property
    def cors_origins(self) -> List[str]:
        if self.allowed_origins.strip() == "*":
            return ["*"]
        return [o.strip() for o in self.allowed_origins.split(",") if o.strip()]


_settings: Optional[Settings] = None


def get_settings() -> Settings:
    global _settings
    if _settings is None:
        _settings = Settings()
    return _settings


# ---------------------------------------------------------------------------
# Runtime bot configuration (stored in DB, editable from dashboard)
# Default seeds come from env so Render env-vars still work as first values.
# ---------------------------------------------------------------------------

DEFAULT_CONFIG: dict = {
    "trading_mode": os.getenv("TRADING_MODE", "paper"),
    "paper_data_source": os.getenv("PAPER_DATA_SOURCE", "testnet"),
    "start_balance": float(os.getenv("START_BALANCE", "10000")),
    "trading_symbols": os.getenv("TRADING_SYMBOLS", "BTCUSDT,ETHUSDT,SOLUSDT,BNBUSDT"),
    "timeframe": os.getenv("TIMEFRAME", "5m"),
    "strategy": os.getenv("STRATEGY", "sma_cross"),
    "stake_mode": os.getenv("STAKE_MODE", "fixed"),
    "stake_amount": float(os.getenv("STAKE_AMOUNT", "100")),
    "stake_percent": float(os.getenv("STAKE_PERCENT", "10")),
    "stop_loss_pct": float(os.getenv("STOP_LOSS_PCT", "2")),
    "take_profit_pct": float(os.getenv("TAKE_PROFIT_PCT", "4")),
    "max_open_trades": int(os.getenv("MAX_OPEN_TRADES", "3")),
    "poll_interval": int(os.getenv("POLL_INTERVAL", "15")),
    "trailing_stop": os.getenv("TRAILING_STOP", "false").lower() in ("1", "true", "yes"),
    "trailing_stop_pct": float(os.getenv("TRAILING_STOP_PCT", "1")),
    "grid_levels": int(os.getenv("GRID_LEVELS", "4")),
    # runtime state (not user-facing)
    "bot_running": True,
    "reset_requested": False,
}

CREDENTIAL_KEYS = ("binance_api_key", "binance_api_secret")


def seed_credentials() -> dict:
    """Credentials never go through the dashboard; seed from env only."""
    out = {}
    if os.getenv("BINANCE_API_KEY"):
        out["binance_api_key"] = os.getenv("BINANCE_API_KEY")
    if os.getenv("BINANCE_API_SECRET"):
        out["binance_api_secret"] = os.getenv("BINANCE_API_SECRET")
    return out


def db_url() -> str:
    """SQLite by default; DATABASE_URL (postgres) if provided (Render disk/Neon)."""
    url = os.getenv("DATABASE_URL", "")
    if url:
        # Render's postgres driver name is sometimes 'postgres://'
        if url.startswith("postgres://"):
            url = url.replace("postgres://", "postgresql://", 1)
        return url
    return f"sqlite:///{(DATA_DIR / 'bot.db').as_posix()}"


def serialize_config(cfg: dict) -> str:
    return json.dumps(cfg, default=str)
