# Operations

Purpose: daily ops and incident response. Audience: humans.

## Change booking day

Preferred (Telegram): `/setday monday`. Bot updates `GPROP_TARGET_DAY` and crontab.

Manual fallback: update `GPROP_TARGET_DAY` in server `.env`, set crontab
`0 0 * * 1` (1=Monday).

## Update booking plan

Edit `GPROP_BOOKING_PLAN` in server `.env`:

```
GPROP_BOOKING_PLAN=07:00-08:00>7935,7937,7936;08:00-09:00>7937,7936,7935
```

## Rotate Telegram token

Message @BotFather, revoke, copy new token, update local + server `.env`,
restart service.

## Troubleshooting

- Login fail: check `GPROP_EMAIL`/`GPROP_PASSWORD`, run `health-check`, see Telegram alert.
- Slot taken: expected at contention; check `court-bot.log` fire/delay fields.
- Cron TZ: server uses Asia/Kuala_Lumpur; do not rely on `CRON_TZ`.
