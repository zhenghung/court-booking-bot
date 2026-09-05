# Configuration

Purpose: full config reference. Audience: humans.

## Court names

`GPROP_FACILITY_IDS` and `GPROP_BOOKING_PLAN` accept numeric IDs (e.g. `7935`)
or names (e.g. `Pickleball Court P1`). Case-insensitive partial match
(e.g. `P1` matches `Pickleball Court P1`).

## Booking plan format

`slot>court1,court2;slot>court1,court2`

Example:

- `07:00-08:00>7935,7937,7936`
- `08:00-09:00>7937,7936,7935`

## Multi-account (optional)

Use `GPROP_ACCOUNT_N_*` variables for multiple accounts (N = 1, 2, 3...):
| Variable | Description |
|----------|-------------|
| `GPROP_ACCOUNT_N_NAME` | Display name for account |
| `GPROP_ACCOUNT_N_EMAIL` | Login email |
| `GPROP_ACCOUNT_N_PASSWORD` | Login password |
| `GPROP_ACCOUNT_N_UNIT_ID` | Unit ID (falls back to global) |
| `GPROP_ACCOUNT_N_BOOKING_NAME` | Booking name (falls back to global) |
| `GPROP_ACCOUNT_N_CONTACT` | Contact (falls back to global) |
| `GPROP_ACCOUNT_N_BOOKING_PLAN` | Account-specific booking plan |

Each account can book up to 2 hours/week, so 2 accounts = 4 hours total.

## Sniping schedules (optional)

Copy `schedules.example.yaml` to `schedules.yaml` (or set
`GPROP_SCHEDULES_FILE` to its path). Each schedule is independent: own
`target_day`, `booking_plan` (same `slot>courts;...` syntax), and `accounts`
(`[all]` or `GPROP_ACCOUNT_N_NAME` values). Schedule names are slugs
`[a-z0-9-]` for `--schedule` and cron. The schedule's plan wins; the
account's own plan is ignored in schedule mode. No file = legacy
`GPROP_TARGET_DAY` + booking-plan path, unchanged.

## Web UI

| Variable | Description | Required |
|----------|-------------|----------|
| `UI_PASSWORD` | Admin password for `./court-bot serve` login | Yes (to serve) |
| `UI_PORT` | Port to listen on (default `8080`) | No |
| `UI_BIND` | Bind address (default `0.0.0.0`) | No |
