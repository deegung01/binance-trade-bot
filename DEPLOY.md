# Hướng dẫn sau-deploy

## 1. Push code lên GitHub (cần PAT — bạn tự tạo)

1. Tạo PAT: https://github.com/settings/tokens → **Generate new token (classic)**
   - Scope: `repo` (đủ để push private/public repo)
2. Lưu token vào Hermes (chỉ secrets mới nằm trong .env):

   ```
   hermes env set GITHUB_PERSONAL_ACCESS_TOKEN ghp_xxx
   ```

   Hoặc edit `~/AppData/Local/hermes/.env` (HERMES_HOME/.env), thêm dòng:
   `GITHUB_PERSONAL_ACCESS_TOKEN=ghp_xxx`

3. Config git credential một lần để push không hỏi mật khẩu:

   ```bash
   git config --global credential.helper store
   # push lần đầu, user: <github-username>, password: <PAT>
   ```

4. Tạo repo + push:

   ```bash
   cd ~/binance-trade-bot
   git remote add origin https://github.com/<username>/binance-trade-bot.git
   git push -u origin master
   ```

   (Tạo repo trên github.com/New repository trước, hoặc dùng MCP github sau khi có token.)

5. Gắn PAT vào MCP GitHub đã cài (sửa `~/AppData/Local/hermes/config.yaml`):

   ```yaml
   github:
     command: npx
     args:
       - -y
       - '@modelcontextprotocol/server-github'
     env:
       GITHUB_PERSONAL_ACCESS_TOKEN: ghp_xxx
     enabled: true
   ```

   Khởi động lại Hermes → dùng được 26 tools GitHub (create repo, push files, PRs, issues…).

## 2. Deploy backend lên Render (sau khi repo đã trên GitHub)

Render → New → Blueprint → chọn repo `binance-trade-bot` → render.yaml tự cấu hình:
- Python 3.11, region Singapore, plan Free
- Root: `backend/` — start: `uvicorn app.main:app --host 0.0.0.0 --port $PORT`
- Health check `/health`

Sau deploy: `https://<tên>.onrender.com`

Chống sleep: cron-job.org → GET `https://<tên>.onrender.com/health` mỗi 10 phút.

## 3. Gắn backend URL vào Vercel

Vercel project `binance-trade-bot` → Settings → Environment Variables:
- `NEXT_PUBLIC_API_URL` = `https://<tên>.onrender.com` → Redeploy

## 4. LIVE mode (không bắt buộc)

Binance testnet keys: https://testnet.binance.vision (login GitHub) → Settings → API keys → dán vào dashboard Settings.
