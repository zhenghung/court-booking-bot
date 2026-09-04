# Docs Restructure Design

Date: 2026-09-04
Status: approved (approach A)
Goal: reduce overlap. Audience: humans + agents.

## Problems

- Config defined 3x: README, AGENTS.md, .env.example.
- Cron/deploy defined 2x: RUNBOOK.md, AGENTS.md. Drift: RUNBOOK stale SSH paths.
- README single-account only. Missing bot/facilities/health-check/Telegram usage.
- PLAN.md stale (booking endpoint TODOs done). No index, no troubleshooting.

## Structure (approved)

- `README.md` (~40 lines): pitch + quickstart + docs index. No config/cron dupes.
- `docs/setup.md`: install + `.env`. Single config table. `.env.example` code truth.
- `docs/usage.md`: all 7 CLI + 4 Telegram commands.
- `docs/configuration.md`: court names, booking plan, multi-account.
- `docs/deployment.md`: server, systemd, cron. Fix stale SSH paths.
- `docs/operations.md`: logs, /status, change day/plan, rotate token, troubleshooting.
- `docs/architecture.md`: move current file + code map + API endpoints.
- `AGENTS.md` (~60 lines): build/test/deploy cmds + docs pointers + agent-only notes.
- Delete: `PLAN.md`, `RUNBOOK.md`, root `ARCHITECTURE.md` (after moves).

## Content mapping (approved)

See structure above. Moves are verbatim except: merge config tables once,
merge cron tables once, fix `ssh-key-*.key` paths, add missing command docs
and troubleshooting (login fail, slot taken, cron TZ).

## Rules + validation (approved)

- `docs/` = human truth. `AGENTS.md` = pointer. `.env.example` = config truth.
- Each doc: 1-line purpose + audience. Cross-link, never copy.
- Validate: `go build ./...`, grep `GPROP_TARGET_DAY` single definition,
  no `Documents/personal` paths, mermaid fences intact, links resolve.

## Out of scope

No site generator. No versioned docs. No new diagrams.
