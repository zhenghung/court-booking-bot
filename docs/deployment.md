# Deployment

Purpose: deploy and schedule the bot on Oracle Cloud. Audience: humans.

## Server

- Host: `ubuntu@149.118.140.17`
- Platform: Oracle Cloud Free Tier ARM64 (Ubuntu 22.04)
- Timezone: Asia/Kuala_Lumpur (UTC+8)
- SSH key: `ssh-key-*.key` in project root (gitignored)

## Deploy

```bash
GOOS=linux GOARCH=arm64 go build -o court-bot-linux-arm64 ./cmd/bot
scp -i ssh-key-*.key court-bot-linux-arm64 ubuntu@149.118.140.17:/home/ubuntu/court-bot
ssh -i ssh-key-*.key ubuntu@149.118.140.17 "chmod +x ~/court-bot && ./court-bot ping"
```

## Cron

| Schedule | Command | Purpose |
|----------|---------|---------|
| `0 0 * * 5` | `./court-bot run --now` | Booking snipe Friday 00:00 MYT |
| `0 8 * * *` | `./court-bot health-check` | Daily login check, alerts on failure only |

Server paths: binary `/home/ubuntu/court-bot`, env `/home/ubuntu/.env`,
logs `/home/ubuntu/court-bot.log`.
