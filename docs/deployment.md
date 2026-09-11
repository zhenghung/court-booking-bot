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
| `59 23 * * *` | `GPROP_SCHEDULES_FILE=/home/ubuntu/.schedules.yaml ./court-bot run >> /home/ubuntu/court-bot.log 2>&1` | Daily file-DB snipe 23:59 MYT — poll 500ms from 23:59:55, target = next midnight +7d (KL), skips if no schedule matches |
| `0 8 * * *` | `./court-bot health-check >> /home/ubuntu/health-check.log 2>&1` | Daily login check, alerts on failure only |

Cron is daily 23:59; weekday comes from each schedule's `target_day` in `~/.schedules.yaml` (change via `/setday <schedule> <day>` or editing the file — no crontab edit needed, see [operations](operations.md)). `--now` is for manual testing only.

Server paths: binary `/home/ubuntu/court-bot`, env `/home/ubuntu/.env`,
schedules `/home/ubuntu/.schedules.yaml`, logs `/home/ubuntu/court-bot.log`.

Cohost note: the VM also runs bank-dashboard (own processes, own cron
line). Edit crontab surgically — only court lines, never a full rewrite.

## Web UI (behind Caddy HTTPS)

The UI is served via Caddy (`https://court.149-118-140-17.sslip.io` →
`127.0.0.1:8080`). Never expose `:8080` directly (no Oracle ingress rule,
no iptables rule). `UI_BIND=127.0.0.1` in server `.env` enforces loopback-only.

Install as a systemd unit (fixes the manual-daemon reboot gap):

```bash
sudo cp deploy/court-serve.service /etc/systemd/system/court-serve.service
sudo systemctl daemon-reload
sudo systemctl enable --now court-serve
systemctl is-active court-serve
ss -tlnp | grep 8080  # expect 127.0.0.1:8080 only
```

Set `UI_PASSWORD` in server `.env` (long random value, 16+ chars, never on the
command line — it leaks via shell history and `ps`).

Caddy trusts `X-Forwarded-For` only from loopback (`internal/web/server.go`):
rate-limiting sees real client IPs through the proxy, WAN-spoofed headers
are ignored.

See [usage](usage.md) for the console tour and [configuration](configuration.md)
for `UI_*` vars.
