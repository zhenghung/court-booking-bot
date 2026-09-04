# Court Booking Bot

Simple Go CLI bot for booking court slots on `gpropsystems.com`.

## Quickstart

```bash
go build -o court-bot ./cmd/bot/
./court-bot ping
./court-bot probe --date 2026-03-04
```

## Docs

| Doc | What |
|-----|------|
| [setup](docs/setup.md) | Install + `.env` |
| [usage](docs/usage.md) | CLI + Telegram commands |
| [configuration](docs/configuration.md) | Courts, plan, multi-account |
| [deployment](docs/deployment.md) | Server + cron |
| [operations](docs/operations.md) | Ops + troubleshooting |
| [architecture](docs/architecture.md) | Flows + code map |

## Security

- Never commit `.env`.
- Never commit private SSH keys.
- Rotate Telegram bot token if exposed.
