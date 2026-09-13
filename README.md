# Binance Testnet Trading Bot

Bot giao dịch tự động trên **Binance Spot Testnet** + dashboard web chi tiết, tham khảo kiến trúc từ **freqtrade** (strategy plug-in, engine loop, risk config) và **OctoBot** (dashboard realtime).

## Kiến trúc

```
binance-trade-bot/
├── backend/                  → deploy lên Render (free 500MB)
│   ├── app/
│   │   ├── main.py           # FastAPI app + start engine
│   │   ├── config.py         # env config + defaults
│   │   ├── database.py       # SQLAlchemy (SQLite/Postgres)
│   │   ├── models.py         # Trade, OrderLog, EquityPoint, LogEntry, BotConfig
│   │   ├── exchange/
│   │   │   └── binance.py    # REST client testnet + PaperClient (mô phỏng ví)
│   │   ├── strategy/
│   │   │   ├── indicators.py # SMA/EMA/RSI/MACD/BB/ATR thuần Python
│   │   │   └── strategies.py # 4 strategies kiểu freqtrade plug-in
│   │   ├── engine/
│   │   │   └── engine.py     # trading loop: signals → SL/TP → equity
│   │   └── api/
│   │       └── routes.py     # REST API cho dashboard
│   ├── requirements.txt
│   ├── keep_alive.py         # chống sleep Render free
│   └── .env.example
├── frontend/                 → deploy lên Vercel
│   ├── app/
│   │   ├── page.js           # Overview: equity curve, stats, positions
│   │   ├── chart/            # Candlestick TradingView-style + trade markers
│   │   ├── trades/           # Bảng lệnh chi tiết + đóng lệnh manual
│   │   ├── strategy/         # Chi tiết strategies + manual buy
│   │   ├── logs/             # Engine logs + order activity realtime
│   │   └── settings/         # Config + API keys + reset wallet
│   ├── components/           # Sidebar, EquityChart
│   └── lib/api.js            # API client (NEXT_PUBLIC_API_URL)
├── render.yaml               # deploy backend 1-click
└── README.md
```

## Backend (Render)

### Chạy local

```bash
cd backend
pip install -r requirements.txt
cp .env.example .env          # chỉnh nếu muốn (mặc định paper mode)
python -m uvicorn app.main:app --port 8000
```

API docs: http://localhost:8000/docs

### Deploy lên Render free

1. Push code lên GitHub repo.
2. Render → **New → Blueprint** → chọn repo (render.yaml tự cấu hình: Python, region Singapore, rootDir backend, health check /health).
   - Hoặc **New → Web Service** thủ công:
     - Runtime: **Python 3**
     - Build: `pip install -r backend/requirements.txt`
     - Start: `uvicorn app.main:app --host 0.0.0.0 --port $PORT`
     - Root directory: `backend`
     - Plan: **Free**
3. Sau deploy bạn có URL dạng `https://<tên>.onrender.com`.

> ⚠️ **Render free sleep sau 15 phút không có request.** Engine chạy bằng thread trong process nên khi sleep bot dừng. Giải pháp: cron-job.org (miễn phí) → tạo cron GET `https://<tên>.onrender.com/health` mỗi 10 phút. SQLite nằm trong `/data` — bản free filesystem là ephemeral (mất khi restart/redeploy), chấp nhận được cho testnet bot; nếu muốn giữ history thì attach Render Postgres (paid) hoặc Neon free và set `DATABASE_URL`.

> 💡 Lưu ý 500MB disk: backend (code + deps ~150MB) nhẹ hơn nhiều giới hạn. SQLite chỉ tăng vài MB mỗi ngày log.

## Frontend (Vercel)

```bash
cd frontend
npm install
NEXT_PUBLIC_API_URL=http://localhost:8000 npm run dev
```

### Deploy lên Vercel

1. Vercel → **Add New → Project** → import cùng GitHub repo.
2. Cấu hình:
   - **Root Directory**: `frontend`
   - Framework preset: Next.js (tự nhận)
3. Vậy là đủ — môi trường production sẽ set env sau.
4. Vào **Settings → Environment Variables** thêm:
   - `NEXT_PUBLIC_API_URL` = `https://<tên-backend>.onrender.com`
   (Production + Preview + Development đều chọn)
5. **Redeploy** để env có hiệu lực.

## Sử dụng

- Vào dashboard → **Settings**: chọn mode `paper` (mặc định, không cần API key) hoặc `live` (cần API key từ [testnet.binance.vision](https://testnet.binance.vision), login GitHub).
- **Strategy**: chọn 1 trong 4 strategies (SMA Cross, EMA Cross + RSI, RSI Reversion, MACD Flip), hoặc mua manual.
- **Overview**: theo dõi equity curve, win rate, drawdown, positions — tự refresh 5s.
- **Chart**: nến realtime + markers lệnh đã vào.
- **Trades**: đóng lệnh bất kỳ lúc nào.

## API chính

| Method | Endpoint | Chức năng |
|---|---|---|
| GET | /api/status | Tổng quan equity + stats + positions |
| GET | /api/candles?symbol=&interval= | Nến cho chart |
| GET | /api/trades, /api/logs, /api/activity, /api/equity | Dữ liệu dashboard |
| POST | /api/config | Đổi config (strategy, symbols, SL/TP…) |
| POST | /api/buy, /api/sell/{id} | Đặt/đóng lệnh manual |
| POST | /api/reset | Reset paper wallet |
| GET | /api/account | Số dư testnet (live mode) |

## Roadmap

- Backtesting (tái hiện strategy trên lịch sử nến)
- WebSocket streams thay REST polling
- Short/margin mode
- Đa chiến lược song song trên cùng symbol
