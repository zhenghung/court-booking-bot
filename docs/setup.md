# Setup

Purpose: get a new operator from clone to first probe. Audience: humans.

## Requirements

- Go 1.22+

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
| `GPROP_TARGET_DAY` | Day of week to book (e.g. "friday") | Yes |
| `GPROP_BOOKING_PLAN` | Slots and court priority | Yes |
| `GPROP_TELEGRAM_BOT_TOKEN` | Telegram bot token | No (for notifications) |
| `GPROP_TELEGRAM_CHAT_ID` | Telegram chat/group ID | No (for notifications) |

See [configuration](configuration.md) for court names, booking plan format, and multi-account.

## Build

```bash
go build -o court-bot ./cmd/bot/
```

## First probe

```bash
./court-bot ping
./court-bot probe --date 2026-03-04
```
