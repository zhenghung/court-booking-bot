# Setup

Purpose: get a new operator from clone to first probe. Audience: humans.

## Requirements

- Go 1.25+ (`go.mod` pins `1.25.0`)

## Configure

Copy `.env.example` to `.env` and fill values. Field reference (single source):

| Variable | Description | Required |
|----------|-------------|----------|
| `GPROP_EMAIL` | Login email | Yes |
| `GPROP_PASSWORD` | Login password | Yes |
| `GPROP_FACILITY_IDS` | Comma-separated court IDs or names | Yes |
| `GPROP_UNIT_ID` | Unit/apartment ID | Yes |
| `GPROP_BOOKING_NAME` | Name for booking | Yes |
| `GPROP_CONTACT` | Contact number | Yes |
| `GPROP_TARGET_DAY` | Day of week to book (e.g. "friday") | Yes, legacy path (not needed with `schedules.yaml`) |
| `GPROP_BOOKING_PLAN` | Slots and court priority | Yes, legacy path (schedule file overrides per-schedule) |
| `GPROP_BASE_URL` | gprop base URL | No |
| `GPROP_SCHEDULES_FILE` | Absolute path to `schedules.yaml` | No (file-DB mode; server cron sets it) |
| `GPROP_ACCOUNT_N_*` | Multi-account `EMAIL/PASSWORD/UNIT_ID/...` (`N = 1,2,3...`, falls back to globals) | No |
| `UI_PASSWORD` | Admin password for `./court-bot serve` (16+ chars) | Yes, to serve |
| `UI_PORT` | Web UI port (default `8080`) | No |
| `UI_BIND` | Web UI bind (default `0.0.0.0`; prod `127.0.0.1` behind Caddy) | No |
| `GPROP_TELEGRAM_BOT_TOKEN` | Telegram bot token | No (for notifications) |
| `GPROP_TELEGRAM_CHAT_ID` | Telegram chat/group ID | No (for notifications) |

See [configuration](configuration.md) for court names, booking plan format, multi-account,
and optional sniping schedules (`schedules.yaml`).

## Build

```bash
go build -o court-bot ./cmd/bot/
```

## First probe

```bash
./court-bot ping
./court-bot probe --date 2026-03-04
```
