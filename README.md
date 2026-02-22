# Court Booking Bot

Simple Go CLI bot for booking court slots on `gpropsystems.com`.

## Features

- Login with CSRF/session handling
- Probe timeslot availability across courts
- Book a single slot (`book` command)
- Scheduled auto-booking (`run` command)
- Per-slot court priority via booking plan
- Optional Telegram notifications

## Requirements

- Go 1.22+
- `.env` file with credentials/config

## Configuration

Copy `.env.example` to `.env` and fill values:

```env
GPROP_EMAIL=your_email@example.com
GPROP_PASSWORD=your_password
GPROP_FACILITY_IDS=7935,7936,7937
GPROP_UNIT_ID=1-135735
GPROP_BOOKING_NAME=YOUR NAME
GPROP_CONTACT=60123456789
GPROP_TARGET_DAY=wednesday
GPROP_BOOKING_PLAN=07:00-08:00>7935,7937,7936;08:00-09:00>7937,7936,7935

# Optional
GPROP_TELEGRAM_BOT_TOKEN=
GPROP_TELEGRAM_CHAT_ID=
```

### Booking plan format

`slot>court1,court2;slot>court1,court2`

Example:

- `07:00-08:00>7935,7937,7936`
- `08:00-09:00>7937,7936,7935`

## Build

```bash
go build -o court-bot ./cmd/bot/
```

## Usage

```bash
./court-bot ping
./court-bot probe --date 2026-03-04
./court-bot book --time 07:00-08:00 --date 2026-03-04
./court-bot run --now --dry-run
./court-bot run
```

## Scheduler notes

- `run` computes target booking date as **today + 7 days**.
- Recommended cron at midnight of your target release day:

```cron
0 0 * * 3 cd /home/ubuntu && ./court-bot run --now >> /home/ubuntu/court-bot.log 2>&1
```

(Above example is Wednesday midnight.)

## Security

- Never commit `.env`.
- Never commit private SSH keys.
- Rotate Telegram bot token if exposed.
