# Hosibot (Go)

Hosibot is the Go implementation of the Hosibot backend and Telegram bot runtime.
It exposes legacy-compatible API routes, webhook handling, payment callbacks, cron jobs, and panel integrations.

## Project layout

- `go/cmd/main.go`: application entrypoint
- `go/internal/`: bot, handlers, router, repositories, config, panel adapters
- `go/deploy.sh`: deployment manager (interactive + CLI flags)
- `go/.env.example`: environment template

## Requirements

- Linux server (systemd recommended)
- MySQL/MariaDB
- Redis
- Telegram bot token
- Public domain + TLS for webhook mode

## Local development

```bash
cd go
cp .env.example .env
# edit .env
go mod download
go run ./cmd
```

Health check:

```bash
curl -s http://127.0.0.1:${APP_PORT:-8080}/health
```

## Environment configuration

At minimum, set these in `go/.env`:

- `APP_PORT`
- `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASS`
- `REDIS_ADDR`, `REDIS_PASS`, `REDIS_DB`
- `BOT_TOKEN`
- `BOT_DOMAIN`
- `BOT_WEBHOOK_URL`
- `API_KEY`
- `JWT_SECRET`

See full template in `go/.env.example`.

## Deploy with `deploy.sh`

### Option A: From cloned repository

```bash
cd /path/to/hosibot/go
chmod +x deploy.sh
./deploy.sh
```

### Option B: Download deploy script with curl

```bash
mkdir -p /opt/hosibot && cd /opt/hosibot
curl -fsSL -o deploy.sh https://raw.githubusercontent.com/FakharzadehH/hosibot/main/go/deploy.sh
chmod +x deploy.sh
./deploy.sh
```

## Recommended deployment flow

Run menu option `1) Quick deploy (recommended)`.

It walks through:

1. Install dependencies (curl/git/jq/python3 + MySQL/MariaDB + Redis)
2. `.env` wizard
3. Download latest release binary
4. Install/update systemd service
5. Start service
6. Set Telegram webhook
7. Run diagnostics (`/health`, DB, Redis)

## Non-interactive deploy commands

```bash
cd go
./deploy.sh --quick-deploy
./deploy.sh --build
./deploy.sh --set-webhook
./deploy.sh --health
```

## Service and runtime management

From deploy menu:

- `6) Background process manager`
- `7) Systemd service manager`
- `8) Telegram webhook manager`
- `9) Diagnostics / health check`

If service is installed, default name is `hosibot.service`.

## API and webhook endpoints

- Health: `GET /health`
- Legacy-compatible API base: `/api/*`
- Bot webhook routes:
  - `POST /bot/webhook`
  - `POST /webhook/:token` (legacy route)
- Payment callbacks:
  - `/payment/zarinpal/callback`
  - `/payment/nowpayments/callback`
  - `/payment/tronado/callback`
  - `/payment/iranpay/callback`
  - `/payment/aqayepardakht/callback`

## Updating

To update binary to latest release:

```bash
cd go
./deploy.sh --build
```

Then restart from service manager (menu option `7`) or:

```bash
sudo systemctl restart hosibot.service
```
