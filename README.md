# Binance Testnet Trading Bot (Go + Next.js)

Bot giao dịch tự động trên **Binance Spot Testnet** + dashboard web, kiến trúc tham khảo **freqtrade** (strategy plug-in, engine loop) và **OctoBot** (dashboard realtime).

## Stack

| Thành phần | Công nghệ | Deploy |
|---|---|---|
| Backend | **Go** (net/http stdlib, zero external deps) — binary ~8MB | Render Web Service (free) |
| Frontend | Next.js 14 + Tailwind + lightweight-charts + recharts | Vercel |
| Storage | JSON files (config/state/trades/logs) — không cần DB | ephemeral disk |

**Tại sao Go?** Binary tĩnh nhỏ (~8MB so với Python venv 200MB+), khởi động <1s, memory ~20MB, không cần pip install khi build — rất hợp Render free 512MB.

## Cấu trúc

```
binance-trade-bot/
├── backend-go/               → Render Web Service
│   ├── cmd/server/main.go    # entrypoint (PORT, DATA_DIR env)
│   └── internal/
│       ├── api/api.go        # REST API (CORS sẵn)
│       ├── config/config.go  # config + state + JSON store
│       ├── engine/engine.go  # trading loop: signals → SL/TP/trailing/grid
│       ├── exchange/binance.go # Binance REST client (testnet + mainnet data)
│       ├── strategy/strategy.go # 5 strategies + adaptive grid
│       └── ta/ta.go          # SMA/EMA/RSI/MACD/ATR
├── frontend/                 → Vercel
│   └── app/                  # Overview, Chart, Trades, Strategy, Logs, Settings
└── render.yaml               # tham khảo config Web Service
```

## Chạy local

```bash
# Backend (cần Go 1.22+)
cd backend-go
go build -o app ./cmd/server
DATA_DIR=./data PORT=8080 ./app

# Frontend
cd frontend
npm install
NEXT_PUBLIC_API_URL=http://localhost:8080 npm run dev
```

Test: `cd backend-go && go test ./...`

## Deploy

### Backend → Render Web Service (free)

Render **không có Blueprint ở free plan** — tạo Web Service thủ công:

1. Push code lên GitHub (repo này)
2. Render → **New + → Web Service** → connect repo
3. Cấu hình:
   - **Root Directory**: `backend-go`
   - **Runtime**: Go (tự nhận `go.mod`)
   - **Build Command**: `go build -o app ./cmd/server`
   - **Start Command**: `./app`
   - **Instance Type**: Free
4. Env vars (tùy chọn, đã có default): `TRADING_MODE`, `TRADING_SYMBOLS`, `TIMEFRAME`, `STRATEGY`, `STOP_LOSS_PCT`, `TAKE_PROFIT_PCT`, `TRAILING_STOP`, `GRID_LEVELS`... Keys live mode: `BINANCE_API_KEY`, `BINANCE_API_SECRET`

⚠️ Free plan **sleep sau 15 phút** không request → cron-job.org ping `https://<service>.onrender.com/health` mỗi 10 phút. Engine chạy bằng goroutine nên khi sleep bot dừng.

### Frontend → Vercel

1. Vercel → Add New Project → import cùng repo
2. **Root Directory**: `frontend`
3. Env: `NEXT_PUBLIC_API_URL` = `https://<service>.onrender.com` → Redeploy

## Strategies

| ID | Logic |
|---|---|
| `sma_cross` | SMA 10/50 golden cross |
| `ema_cross` | EMA 9/21 cross + RSI > 50 |
| `rsi_revert` | RSI oversold hồi phục (30/70) |
| `macd` | MACD histogram flip âm → dương |
| `adaptive_grid` | Grid spacing theo ATR (clamp 0.5–4%), DCA khi giá tụt 1 spacing, SL/TP rebase theo avg cost |

Tất cả đều có: Stop Loss, Take Profit, **Trailing Stop** (ratchet-only), và max N lệnh mở / 1 lệnh mỗi symbol.

## API

| Endpoint | Chức năng |
|---|---|
| `GET /api/status` | equity, stats, positions, last cycle |
| `GET /api/candles?symbol=&interval=&limit=` | nến cho chart |
| `GET /api/trades, /logs, /activity, /equity` | dữ liệu dashboard |
| `GET/POST /api/config` | đọc/đổi config |
| `POST /api/buy` `{symbol, stake}` | mua manual |
| `POST /api/sell/{id}` | đóng lệnh |
| `POST /api/reset` | reset paper wallet |
| `GET /health` | health check |
