# Agent Instructions

Bot: Go CLI for gpropsystems court booking. Cron + Telegram on Oracle ARM VM.

## Build / test

```bash
go build -o court-bot ./cmd/bot
go test ./...
./court-bot ping
./court-bot probe --date 2026-03-06
GOOS=linux GOARCH=arm64 go build -o court-bot-linux-arm64 ./cmd/bot
```

## Docs (single source)

- docs/setup.md: install + .env
- docs/usage.md: CLI + Telegram
- docs/configuration.md: courts, plan, multi-account
- docs/deployment.md: server + cron
- docs/operations.md: ops + troubleshooting
- docs/architecture.md: flows + code map

Rules: docs/ is human truth, .env.example is config truth. Cross-link, never copy.

## Gotchas

- Cron uses Asia/Kuala_Lumpur; no CRON_TZ.
- Never commit `.env` or `ssh-key-*.key`.
- Target date = today + 7d. Never book before midnight in `run`.
