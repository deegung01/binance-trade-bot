"""Binance Testnet Trading Bot — FastAPI application entrypoint."""
from __future__ import annotations

from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.config import get_settings
from app.database import Base, engine
from app.engine.engine import engine as trading_engine
from app.api.routes import router


@asynccontextmanager
async def lifespan(app: FastAPI):
    # create tables
    Base.metadata.create_all(bind=engine)
    # lightweight migration: add Trade.meta column to pre-existing DBs
    try:
        from sqlalchemy import inspect, text
        with engine.connect() as conn:
            cols = [c["name"] for c in inspect(conn).get_columns("trades")]
            if "meta" not in cols:
                conn.execute(text("ALTER TABLE trades ADD COLUMN meta JSON"))
                conn.commit()
    except Exception:
        pass
    # start trading loop
    trading_engine.start()
    yield
    trading_engine.stop()


app = FastAPI(
    title="Binance Testnet Trading Bot",
    version="1.0.0",
    description="Paper/live trading bot for Binance Spot Testnet with a dashboard",
    lifespan=lifespan,
)

settings = get_settings()
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origins,
    allow_credentials=False,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(router, prefix="/api")


@app.get("/")
def root():
    return {"ok": True, "service": "binance-trade-bot", "docs": "/docs"}


@app.get("/health")
def health():
    return {"status": "ok"}
