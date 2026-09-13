"""
Keep-alive ping — chống Render free sleep.

Cron-job.org: gọi GET https://<backend>.onrender.com/health mỗi 10 phút
(đặt cron bên ngoài, hoặc import file cronjob.py này để chạy github actions)
"""
import httpx

BACKEND_URL = "https://binance-trade-bot.onrender.com"

if __name__ == "__main__":
    try:
        r = httpx.get(f"{BACKEND_URL}/health", timeout=60)
        print(f"keep-alive: {r.status_code}")
    except Exception as e:
        print(f"keep-alive failed: {e}")
