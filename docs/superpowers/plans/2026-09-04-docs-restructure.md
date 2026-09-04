# Docs Restructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure repo docs into `docs/` with single source of truth, no overlap.

**Architecture:** Move content verbatim from `README.md`, `AGENTS.md`, `RUNBOOK.md` into six `docs/` pages; slim `README.md` to index; slim `AGENTS.md` to pointer; delete `PLAN.md`, `RUNBOOK.md`, root `ARCHITECTURE.md`.

**Tech Stack:** Markdown, Mermaid (existing), Go build for validation only.

**Spec:** `docs/superpowers/specs/2026-09-04-docs-restructure-design.md`

## Global Constraints

- `docs/` is human truth. `AGENTS.md` is pointer. `.env.example` is config truth.
- Each doc starts with 1-line purpose + audience.
- Cross-link, never copy.
- No site generator. No versioned docs. No new diagrams.

---

### Task 1: Setup + configuration pages

**Files:**
- Create: `docs/setup.md`
- Create: `docs/configuration.md`

**Interfaces:**
- Consumes: `README.md:16-47`, `AGENTS.md:142-175`, `.env.example` (all 21 lines)
- Produces: single config table used by later tasks via link (no duplication)

- [ ] **Step 1: Create `docs/setup.md` with purpose line + install + env table**

```markdown
# Setup

Purpose: get a new operator from clone to first probe. Audience: humans.

## Requirements

- Go 1.22+

## Configure

Copy `.env.example` to `.env` and fill values. Field reference (single source):

| Variable | Description | Required |
|----------|-------------|----------|
| `GPROP_EMAIL` | Login email | Yes |
| `GPROP_PASSWORD` | Login password | Yes |
| `GPROP_FACILITY_IDS` | Comma-separated court IDs or names | Yes |
| `GPROP_UNIT_ID` | Unit/apartment ID | Yes |
| `GPROP_BOOKING_NAME` | Name for booking | Yes |
| `GPROP_CONTACT` | Contact number | Yes |
| `GPROP_TARGET_DAY` | Day of week to book (e.g. "friday") | Yes |
| `GPROP_BOOKING_PLAN` | Slots and court priority | Yes |
| `GPROP_TELEGRAM_BOT_TOKEN` | Telegram bot token | No (for notifications) |
| `GPROP_TELEGRAM_CHAT_ID` | Telegram chat/group ID | No (for notifications) |

See [configuration](configuration.md) for court names, booking plan format, and multi-account.

## Build

```bash
go build -o court-bot ./cmd/bot/
```

## First probe

```bash
./court-bot ping
./court-bot probe --date 2026-03-04
```
```

- [ ] **Step 2: Create `docs/configuration.md` with court names + plan + multi-account**

```markdown
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
```

- [ ] **Step 3: Verify files render (head check)**

Run: `head -n 5 docs/setup.md docs/configuration.md`
Expected: both start with `#` title then blank line then `Purpose:` line.

- [ ] **Step 4: Commit**

```bash
git add docs/setup.md docs/configuration.md
git commit -m "Add docs setup and configuration pages"
```

### Task 2: Usage page (CLI + Telegram)

**Files:**
- Create: `docs/usage.md`

**Interfaces:**
- Consumes: `AGENTS.md:21-40` (CLI + Telegram tables), `cmd/bot/main.go` printUsage block
- Produces: command reference linked from README index

- [ ] **Step 1: Create `docs/usage.md` covering all commands**

```markdown
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

```bash
./court-bot ping
./court-bot probe --date 2026-03-04
./court-bot book --time 07:00-08:00 --date 2026-03-04
./court-bot run --now --dry-run
./court-bot run
```

`run` computes target booking date as **today + 7 days**.

## Telegram

| Command | Description |
|---------|-------------|
| `/status` | Check bot config, next run time, booking plan |
| `/setday <day>` | Update booking day and cron day (e.g. `/setday monday`) |
| `/bookings` | Show upcoming bookings from today onwards |
| `/help` | Show help message |
```

- [ ] **Step 2: Verify no command missing vs printUsage**

Run: `grep -c "court-bot" docs/usage.md`
Expected: output `8` or higher (7 table rows + usage block).

- [ ] **Step 3: Commit**

```bash
git add docs/usage.md
git commit -m "Add docs usage page"
```

### Task 3: Deployment + operations pages

**Files:**
- Create: `docs/deployment.md`
- Create: `docs/operations.md`

**Interfaces:**
- Consumes: `RUNBOOK.md` (all 95 lines), `AGENTS.md:41-92` (deployment), `AGENTS.md:185-209` (tasks)
- Produces: single cron table + ops procedures; fixes stale SSH paths

- [ ] **Step 1: Create `docs/deployment.md` (merged, paths fixed)**

```markdown
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
```

- [ ] **Step 2: Create `docs/operations.md` (tasks + troubleshooting)**

```markdown
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
```

- [ ] **Step 3: Verify no stale paths**

Run: `grep -rn "Documents/personal" docs/ || echo "clean"`
Expected: `clean`.

- [ ] **Step 4: Commit**

```bash
git add docs/deployment.md docs/operations.md
git commit -m "Add docs deployment and operations pages"
```

### Task 4: Architecture move + README index

**Files:**
- Create: `docs/architecture.md` (move of `ARCHITECTURE.md` + appendix)
- Modify: `README.md` (slim to index)
- Delete: `ARCHITECTURE.md` (after move)

**Interfaces:**
- Consumes: `ARCHITECTURE.md` (all 198 lines), `AGENTS.md:94-140` (code map + API)
- Produces: README index linking all six docs pages

- [ ] **Step 1: Move architecture file and append code map appendix**

Run: `git mv ARCHITECTURE.md docs/architecture.md`
Expected: `docs/architecture.md` exists, root file gone.

Then append this appendix to `docs/architecture.md`:

```markdown
## Code map

- `cmd/bot/main.go`: CLI commands, Telegram bot daemon
- `internal/api/client.go`: HTTP client (login, booking)
- `internal/config/config.go`: .env loading and parsing

## API endpoints

- `POST /login/login_data_submit`: auth (CSRF `_co6sO0rpsfat` first)
- `POST /booking/get_booking_timeslot`: timeslots per facility + date
- `POST /booking/add_new_booking_action`: create booking (multipart otherData)
- `GET /booking/booking_listing`: bookings from today onwards
```

- [ ] **Step 2: Rewrite `README.md` as slim index**

```markdown
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
```

- [ ] **Step 3: Verify links resolve**

Run: `ls docs/setup.md docs/usage.md docs/configuration.md docs/deployment.md docs/operations.md docs/architecture.md`
Expected: all six listed, no error.

- [ ] **Step 4: Commit**

```bash
git add docs/architecture.md README.md ARCHITECTURE.md
git commit -m "Move architecture to docs, slim README to index"
```

### Task 5: Slim AGENTS.md, delete stale files, final validation

**Files:**
- Modify: `AGENTS.md` (slim to ~60 lines)
- Delete: `PLAN.md`, `RUNBOOK.md`

**Interfaces:**
- Consumes: all six `docs/` pages from Tasks 1-4
- Produces: finished restructure; validation gates pass

- [ ] **Step 1: Rewrite `AGENTS.md` as agent pointer**

```markdown
# Agent Instructions

Bot: Go CLI for gpropsystems court booking. Cron + Telegram on Oracle ARM VM.

## Build / test

```bash
go build -o court-bot ./cmd/bot
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
```

- [ ] **Step 2: Delete stale files**

Run: `git rm PLAN.md RUNBOOK.md && git status --short`
Expected: both staged deleted, plus AGENTS.md modified.

- [ ] **Step 3: Run full validation**

Run: `go build ./... && grep -rn "Documents/personal" docs/ README.md AGENTS.md || echo "paths clean"`
Expected: build succeeds, `paths clean`.

Run: `grep -rln "GPROP_TARGET_DAY" docs/ | sort`
Expected: `docs/configuration.md`, `docs/deployment.md`, `docs/operations.md`, `docs/setup.md` (defined once in setup, referenced elsewhere — no duplicate tables).

- [ ] **Step 4: Commit**

```bash
git add AGENTS.md PLAN.md RUNBOOK.md
git commit -m "Slim AGENTS.md, remove stale PLAN and RUNBOOK"
```
