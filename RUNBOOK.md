# Runbook: Oracle Cloud SSH + Deploy Updates

This is a quick operational guide for future sessions.

## Server details

- Host/IP: `149.118.140.17`
- User: `ubuntu`
- Key file: `/Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key`
- App path on server: `/home/ubuntu/court-bot`
- Env file on server: `/home/ubuntu/.env`
- Log file on server: `/home/ubuntu/court-bot.log`

## 1) SSH into server

```bash
ssh -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key ubuntu@149.118.140.17
```

## 2) Build latest binary (from local project)

```bash
GOOS=linux GOARCH=arm64 go build -o court-bot-linux-arm64 ./cmd/bot/
```

## 3) Copy binary + env to server

```bash
scp -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key \
  /Users/zhenghung.chuah/Documents/personal/court-booking-bot/court-bot-linux-arm64 \
  ubuntu@149.118.140.17:~/court-bot

scp -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key \
  /Users/zhenghung.chuah/Documents/personal/court-booking-bot/.env \
  ubuntu@149.118.140.17:~/.env
```

## 4) Validate on server

```bash
ssh -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key ubuntu@149.118.140.17 \
  "chmod +x ~/court-bot && cd ~ && ./court-bot ping"

ssh -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key ubuntu@149.118.140.17 \
  "cd ~ && ./court-bot run --now --dry-run"
```

## 5) Cron schedule (current)

Current scheduler is set to Friday midnight MYT:

```cron
0 0 * * 5 cd /home/ubuntu && ./court-bot run --now >> /home/ubuntu/court-bot.log 2>&1
```

Preferred way to change booking day:

- In Telegram group, send: `/setday monday` (or any weekday)
- Bot will update both:
  - `GPROP_TARGET_DAY` in `~/.env`
  - user crontab day number

Check/update cron:

```bash
ssh -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key ubuntu@149.118.140.17 "crontab -l"
ssh -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key ubuntu@149.118.140.17 "crontab -e"
```

## 6) Useful checks

```bash
# server time/timezone
ssh -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key ubuntu@149.118.140.17 "date; timedatectl | head -n 8"

# target day in env
ssh -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key ubuntu@149.118.140.17 "grep '^GPROP_TARGET_DAY=' ~/.env"

# recent app logs
ssh -i /Users/zhenghung.chuah/Documents/personal/court-booking-bot/ssh-key-2026-02-22.key ubuntu@149.118.140.17 "tail -n 100 /home/ubuntu/court-bot.log"
```

## Notes / gotchas

- Ubuntu cron here uses server timezone; do not rely on `CRON_TZ`.
- Keep `.env` and `ssh-key-*.key` out of git.
- If Telegram token is rotated, update both local `.env` and server `~/.env`.
