# Agent Instructions

This file helps LLM agents understand and work with this codebase.

## Project Overview

Go CLI bot for automating court bookings on gpropsystems.com. Runs on Oracle Cloud Free Tier ARM64 VM with cron scheduling and Telegram notifications.

## Quick Reference

### Build Commands

```bash
# Build locally (macOS)
go build -o court-bot ./cmd/bot

# Cross-compile for server (Linux ARM64)
GOOS=linux GOARCH=arm64 go build -o court-bot-linux-arm64 ./cmd/bot
```

### CLI Commands

| Command | Description |
|---------|-------------|
| `./court-bot ping` | Test HTTP connectivity to gpropsystems |
| `./court-bot probe --date 2026-03-06` | Check court availability for a date |
| `./court-bot book --time 07:00-08:00` | Book a specific timeslot |
| `./court-bot run --now --dry-run` | Test scheduler without booking |
| `./court-bot bot` | Run Telegram bot daemon |

### Telegram Commands

| Command | Description |
|---------|-------------|
| `/status` | Check bot config, next run time, booking plan |
| `/setday <day>` | Update booking day and cron day (e.g., `/setday monday`) |

## Deployment

### Server Details

- **Host**: `ubuntu@149.118.140.17`
- **Platform**: Oracle Cloud Free Tier ARM64 (Ubuntu 22.04)
- **Timezone**: Asia/Kuala_Lumpur (UTC+8)
- **SSH key**: `ssh-key-*.key` in project root (gitignored)

### File Locations on Server

| Path | Description |
|------|-------------|
| `/home/ubuntu/court-bot` | Binary |
| `/home/ubuntu/.env` | Configuration |
| `/home/ubuntu/court-bot.log` | Cron output log |

### Deploy Updated Binary

```bash
# 1. Stop the service
ssh -i ssh-key-*.key ubuntu@149.118.140.17 "sudo systemctl stop court-bot"

# 2. Copy new binary
scp -i ssh-key-*.key court-bot-linux-arm64 ubuntu@149.118.140.17:/home/ubuntu/court-bot

# 3. Restart service
ssh -i ssh-key-*.key ubuntu@149.118.140.17 "sudo systemctl start court-bot"
```

### Service Management

```bash
# Check status
sudo systemctl status court-bot

# View logs
journalctl -u court-bot -f

# Restart
sudo systemctl restart court-bot
```

### Cron Schedule

- Configured via `crontab -e` on server
- Format: `0 0 * * 5` = Friday 00:00 MYT
- Runs: `./court-bot run --now`

## Code Structure

```
court-booking-bot/
├── cmd/bot/main.go          # CLI commands, Telegram bot daemon
├── internal/
│   ├── api/client.go        # HTTP client for gpropsystems (login, booking)
│   └── config/config.go     # .env loading and parsing
├── .env.example             # Example configuration
├── RUNBOOK.md               # Operational procedures
└── AGENTS.md                # This file
```

### Key Functions in `cmd/bot/main.go`

| Function | Purpose |
|----------|---------|
| `cmdPing()` | Test connectivity |
| `cmdProbe()` | Fetch and display timeslots |
| `cmdBook()` | Book a single slot |
| `cmdRun()` | Scheduler with midnight wait and booking |
| `cmdBot()` | Telegram polling daemon |
| `sendTelegramMessage()` | Send notification to Telegram |

### Key Functions in `internal/api/client.go`

| Function | Purpose |
|----------|---------|
| `Login()` | Authenticate and store session |
| `GetTimeslots()` | Fetch available slots for a date |
| `BookSlot()` | Submit booking request |
| `fetchCSRFToken()` | Extract CSRF token from login page |

## Configuration

Environment variables (in `.env`):

| Variable | Description |
|----------|-------------|
| `GPROP_EMAIL` | Login email |
| `GPROP_PASSWORD` | Login password |
| `GPROP_FACILITY_IDS` | Comma-separated court IDs |
| `GPROP_UNIT_ID` | Unit/apartment ID |
| `GPROP_BOOKING_NAME` | Name for booking |
| `GPROP_CONTACT` | Contact number |
| `GPROP_TARGET_DAY` | Day of week to book (e.g., "friday") |
| `GPROP_BOOKING_PLAN` | Slots and court priority (e.g., `07:00-08:00>7935,7937`) |
| `GPROP_TELEGRAM_BOT_TOKEN` | Telegram bot token |
| `GPROP_TELEGRAM_CHAT_ID` | Telegram chat/group ID |

## Testing Changes

1. Build: `go build -o court-bot ./cmd/bot`
2. Test locally: `./court-bot ping` or `./court-bot probe`
3. Cross-compile: `GOOS=linux GOARCH=arm64 go build -o court-bot-linux-arm64 ./cmd/bot`
4. Deploy (see above)
5. Verify via Telegram `/status` command

## Common Tasks

### Change booking day

Preferred (Telegram):
1. In Telegram group, run: `/setday monday`
2. Bot updates both `GPROP_TARGET_DAY` in `~/.env` and user crontab

Manual fallback:
1. Update server `.env`: `GPROP_TARGET_DAY=monday`
2. Update crontab: `0 0 * * 1` (1=Monday)

### Update booking plan

Edit `GPROP_BOOKING_PLAN` in server `.env`:
```
GPROP_BOOKING_PLAN=07:00-08:00>7935,7937,7936;08:00-09:00>7937,7936,7935
```

### Rotate Telegram bot token

1. Message @BotFather → `/revoke` → select bot
2. Copy new token
3. Update both local `.env` and server `~/.env`
4. Restart service: `sudo systemctl restart court-bot`
