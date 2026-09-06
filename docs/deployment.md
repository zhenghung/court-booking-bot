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
scp -i ssh-key-*.key court-bot-linux-arm64 ubuntu@149.118.140.17:/home/ubuntu/court-bot.new
ssh -i ssh-key-*.key ubuntu@149.118.140.17 "mv ~/court-bot.new ~/court-bot && chmod +x ~/court-bot && ./court-bot ping"
ssh -i ssh-key-*.key ubuntu@149.118.140.17 "cd ~ && ./court-bot run --now --dry-run"
```

Deploy via rename (`court-bot.new` → `court-bot`): direct `scp` onto the
binary fails while the `bot`/`serve` daemons run it (text file busy), and
rename leaves running daemons on the old inode — no restarts needed for
cron-spawned commands.

## Schedules file

```bash
scp -i ssh-key-*.key schedules.yaml ubuntu@149.118.140.17:/home/ubuntu/.schedules.yaml
ssh -i ssh-key-*.key ubuntu@149.118.140.17 "cd ~ && GPROP_SCHEDULES_FILE=/home/ubuntu/.schedules.yaml ./court-bot run --list-schedules"
```

Cron passes `GPROP_SCHEDULES_FILE` inline (absolute path — never rely on
CWD). See [configuration](configuration.md) for the schema.

## Cron

| Schedule | Command | Purpose |
|----------|---------|---------|
| `0 0 * * 5` | `GPROP_SCHEDULES_FILE=/home/ubuntu/.schedules.yaml ./court-bot run --now --schedule fri-pickle` | Booking snipe Friday 00:00 MYT |
| `0 8 * * *` | `./court-bot health-check` | Daily login check, alerts on failure only |

Cron weekday follows each schedule's `target_day` (change via `/setday`, see [operations](operations.md)).

## Cron

| Schedule | Command | Purpose |
|----------|---------|---------|
| `0 0 * * 5` | `GPROP_SCHEDULES_FILE=/home/ubuntu/.schedules.yaml ./court-bot run --now --schedule fri-pickle` | Booking snipe Friday 00:00 MYT |
| `0 8 * * *` | `./court-bot health-check` | Daily login check, alerts on failure only |

Server paths: binary `/home/ubuntu/court-bot`, env `/home/ubuntu/.env`,
schedules `/home/ubuntu/.schedules.yaml`, logs `/home/ubuntu/court-bot.log`.

Cohost note: the VM also runs bank-dashboard (own processes, own cron
line). Edit crontab surgically — only court lines, never a full rewrite.

## Web UI

Set `UI_PASSWORD` in server `.env` (long random value, 16+ chars), open Oracle ingress for
the UI port (`UI_PORT`, default 8080), then run as a daemon:

```bash
./court-bot serve >> /home/ubuntu/court-bot.log 2>&1 &
```

(Serve reads `UI_*` from the server `.env` — never pass the password on the
command line; it leaks via shell history and `ps`.)

See [usage](usage.md) for the console tour and [configuration](configuration.md)
for `UI_*` vars.
