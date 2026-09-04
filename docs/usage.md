# Usage

Purpose: every CLI and Telegram command in one place. Audience: humans.

## CLI

| Command | Description |
|---------|-------------|
| `./court-bot ping` | Test HTTP connectivity to gpropsystems |
| `./court-bot probe --date 2026-03-06` | Check court availability for a date |
| `./court-bot book --time 07:00-08:00` | Book a specific timeslot |
| `./court-bot run --now --dry-run` | Test scheduler without booking |
| `./court-bot bot` | Run Telegram bot daemon |
| `./court-bot facilities` | List all available courts with IDs and names |
| `./court-bot health-check` | Test login functionality and alert on failure |
| `./court-bot serve` | Run embedded web UI (dashboard, bookings, probe, book) |

```bash
./court-bot ping
./court-bot probe --date 2026-03-04
./court-bot book --time 07:00-08:00 --date 2026-03-04
./court-bot run --now --dry-run
./court-bot run
```

`run` computes target booking date as **today + 7 days**.

## Web UI

`UI_PASSWORD=... ./court-bot serve` hosts the ops console (default `:8080`,
see [configuration](configuration.md) for `UI_*` vars and
[deployment](deployment.md) for server setup). Dashboard, bookings, probe grid,
and book (dry-run default, confirm required for live). Avoid 23:59–00:01 on
snipe night — cron owns that window.

## Telegram

| Command | Description |
|---------|-------------|
| `/status` | Check bot config, next run time, booking plan |
| `/setday <day>` | Update booking day and cron day (e.g. `/setday monday`) |
| `/bookings` | Show upcoming bookings from today onwards |
| `/help` | Show help message |
