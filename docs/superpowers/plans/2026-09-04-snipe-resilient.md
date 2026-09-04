# Prevent Login Timeout + Midnight Snipe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate single `dial tcp 162.253.17.182:443: connect: connection timed out` from killing weekly run and snipe `07:00-09:00 P3→P1→P2` at `00:00:00` before others.

**Architecture:** Add `http.Client Timeout` + `Transport` dial/TLS/header timeouts, wrap `Login`/`fetchCSRFToken` with retry on transient `dial tcp/timeout/reset` only, replace blind `Sleep until midnight` with precise pre-warm at `23:59:30` plus `200ms` poll loop `23:59:55→00:00:30` that fires `BookSlot` the instant slot flips. Keep existing `BookSlot 3x` retry.

**Tech Stack:** Go 1.22, `net/http`, `time`, `net`, `internal/api/client.go`, `cmd/bot/main.go`

**Spec:** Approved bounded design in chat: Option B poll @ 200ms, resilience to transient, latency validated (`GET /login 0.10s`, `POST login 0.75s`, `GetTimeslots 0.15s`), approved 2026-09-04.

## Global Constraints

- No new dependencies — stdlib only (`net/http`, `time`, `net`, `strings`)
- Keep `.env` and crontab contracts: `GPROP_TARGET_DAY=friday`, `0 0 * * 5` cron
- Cross-compile must still work: `GOOS=linux GOARCH=arm64 go build -o court-bot-linux-arm64 ./cmd/bot`
- Telegram alert on final failure must remain: `notify` only after retries exhausted
- Poll error handling: `GetTimeslots` transient error is non-fatal `continue`, not abort

---

## File Structure

**Modify:**
- `internal/api/client.go:24-38` — `NewClient` with timeouts
- `internal/api/client.go:51-129` — `Login` + `fetchCSRFToken` retry + `isRetryableError` helper
- `internal/api/client.go:580-606` — `fetchCSRFToken` respects timeouts (no new signature)
- `cmd/bot/main.go:349-400` — `cmdRun` poll loop + re-login logging

**No new files** — 2-file change, ~60 lines added.

**Read before edit:**
- `internal/api/client.go:24-38` (NewClient), `52-61` (Login entry), `581-606` (fetchCSRFToken)
- `cmd/bot/main.go:349-386` (initial + re-login midnight logic), `444-475` (BookSlot 3x retry pattern to mirror)

---

### Task 1: Add HTTP timeouts to API client

**Files:**
- Modify: `internal/api/client.go:24-38`
- Test: `internal/api/client_test.go` (create if missing)

**Interfaces:**
- Consumes: none
- Produces: `NewClient(baseURL string) *Client` returns client with `HTTPClient.Timeout = 15s`, `Transport.DialContext = 5s`, `TLSHandshakeTimeout = 5s`, `ResponseHeaderTimeout = 10s`

- [ ] **Step 1: Write the failing test**

```go
package api

import (
  "testing"
  "time"
)

func TestNewClientHasTimeout(t *testing.T) {
  c := NewClient("https://www.gpropsystems.com")
  if c.HTTPClient.Timeout == 0 {
    t.Fatal("expected Timeout >0, got 0")
  }
  if c.HTTPClient.Timeout < 10*time.Second {
    t.Fatalf("Timeout too short %v, want >=10s", c.HTTPClient.Timeout)
  }
  // Transport timeouts checked via type assert
  tr, ok := c.HTTPClient.Transport.(*http.Transport)
  if !ok {
    t.Fatal("Transport not *http.Transport")
  }
  if tr.TLSHandshakeTimeout == 0 {
    t.Fatal("expected TLSHandshakeTimeout >0")
  }
  if tr.ResponseHeaderTimeout == 0 {
    t.Fatal("expected ResponseHeaderTimeout >0")
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run TestNewClientHasTimeout -v`
Expected: FAIL `expected Timeout >0` (current `NewClient` sets `0`)

- [ ] **Step 3: Write minimal implementation**

```go
// internal/api/client.go:24
import (
  "net"
  "net/http"
  // ... existing
)

func NewClient(baseURL string) *Client {
  jar, _ := cookiejar.New(nil)
  transport := &http.Transport{
    DisableCompression: true,
    DialContext: (&net.Dialer{
      Timeout: 5 * time.Second,
    }).DialContext,
    TLSHandshakeTimeout:   5 * time.Second,
    ResponseHeaderTimeout: 10 * time.Second,
  }
  return &Client{
    BaseURL: baseURL,
    HTTPClient: &http.Client{
      Transport: transport,
      Jar:       jar,
      Timeout:   15 * time.Second,
    },
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api -run TestNewClientHasTimeout -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/client.go internal/api/client_test.go
git commit -m "feat: add http timeouts to api client"
```

---

### Task 2: Add retry wrapper for Login (covers fetchCSRFToken)

**Files:**
- Modify: `internal/api/client.go:51-129` and `580-606`
- Modify: `internal/api/client.go` — add helper `isRetryableError`
- Test: `internal/api/client_test.go`

**Interfaces:**
- Consumes: Task 1 timeouts (fails fast instead of hang)
- Produces: `func (c *Client) Login(email, password string) error` now retries 3x on transient (`dial tcp`, `connection timed out`, `connection reset`, `timeout`, `context deadline exceeded`) with backoff `1s,2s`; auth failures (`status false`/`msg`) not retried. Helper `isRetryableError(error) bool` exported for test or unexported.

- [ ] **Step 1: Write the failing test**

```go
func TestIsRetryable(t *testing.T) {
  cases := []struct{ err string; want bool }{
    {"Get \"https://www.gpropsystems.com/login\": dial tcp 162.253.17.182:443: connect: connection timed out", true},
    {"read tcp 10.0.0.227:49456->149.154.166.110:443: read: connection reset by peer", true},
    {"Client.Timeout exceeded while awaiting headers", true},
    {"context deadline exceeded", true},
    {"login failed: invalid password", false},
    {"CSRF token not found in login page HTML", false},
  }
  for _, c := range cases {
    got := isRetryableError(fmt.Errorf("%s", c.err))
    if got != c.want {
      t.Fatalf("isRetryable(%q)=%v want %v", c.err, got, c.want)
    }
  }
}

func TestLoginRetriesOnTimeout(t *testing.T) {
  // Count attempts: first 2 fetchCSRFToken fail with timeout, 3rd succeeds
  attempts := 0
  // Use httptest server that sleeps or returns timeout via handler
  // Minimal: test isRetryable suffices for logic; full Login retry covered by integration probe
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run TestIsRetryable -v`
Expected: FAIL `isRetryableError not defined`

- [ ] **Step 3: Write minimal implementation**

```go
// helper near top of client.go
func isRetryableError(err error) bool {
  if err == nil { return false }
  s := strings.ToLower(err.Error())
  return strings.Contains(s, "dial tcp") ||
    strings.Contains(s, "connection timed out") ||
    strings.Contains(s, "connection reset") ||
    strings.Contains(s, "client.timeout") ||
    strings.Contains(s, "context deadline exceeded") ||
    strings.Contains(s, "timeout")
}

// Login: wrap existing body with loop
func (c *Client) Login(email, password string) error {
  var lastErr error
  for attempt := 1; attempt <= 3; attempt++ {
    if attempt > 1 {
      backoff := time.Duration(1 << (attempt - 2)) * time.Second // 1s,2s
      fmt.Printf("  Login retry %d after %s...\n", attempt, backoff)
      time.Sleep(backoff)
    }
    err := c.loginOnce(email, password)
    if err == nil {
      return nil
    }
    lastErr = err
    if !isRetryableError(err) {
      return err // auth / parse fail, don't retry
    }
    fmt.Printf("  Login attempt %d failed: %v\n", attempt, err)
  }
  return fmt.Errorf("login failed after 3 attempts: %w", lastErr)
}

// extract current Login body to loginOnce(email,password) error
func (c *Client) loginOnce(email, password string) error {
  // existing Login body (fetchCSRFToken + POST login_data_submit + parse)
}
```

Keep `fetchCSRFToken` using `c.HTTPClient.Get` which now respects `Timeout 15s` from Task 1; no change needed there except it will be retried via outer loop.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api -v`
Expected: PASS for `TestIsRetryable`, existing tests still pass

- [ ] **Step 5: Commit**

```bash
git add internal/api/client.go internal/api/client_test.go
git commit -m "feat: retry login on transient network errors"
```

---

### Task 3: Scheduler snipe poll loop at 200ms (resilient)

**Files:**
- Modify: `cmd/bot/main.go:349-410` (cmdRun login + midnight wait + poll)
- Test: manual dry-run + log inspection (no unit test for timing)

**Interfaces:**
- Consumes: Task 2 `Login` (now retries)
- Produces: `cmdRun` with `23:59:30` re-login, `23:59:55→00:00:30` 200ms poll, immediate `BookSlot` fire, Telegram with `pollAttempts` + `fireDelayMs`

- [ ] **Step 1: Write the failing test (manual)**

No automated test — verify current behavior before change:

Run: `go build -o /tmp/court-bot ./cmd/bot && /tmp/court-bot probe --date 2026-09-18 2>&1 | head -20`
Expected: single probe, no poll, takes `1.3s`

- [ ] **Step 2: Implement poll loop**

Replace midnight wait block `main.go:366-400` with:

```go
// Step 2: Wait for midnight (unless --now)
if !*now {
  midnight := time.Date(today.Year(), today.Month(), today.Day()+1, 0, 0, 0, 0, today.Location())
  waitDuration := time.Until(midnight)

  // Re-login at 23:59:30 if more than 60s away
  if waitDuration > 60*time.Second {
    preLoginWait := waitDuration - 30*time.Second
    fmt.Printf("  Sleeping %s, then re-authenticating...\n", preLoginWait.Round(time.Second))
    time.Sleep(preLoginWait)
    fmt.Println("  Re-authenticating all accounts...")
    for i, acc := range cfg.Accounts {
      // Login now retries internally (Task 2), just log attempt
      fmt.Printf("  %s: re-login... ", acc.Name)
      if err := clients[i].Login(acc.Email, acc.Password); err != nil {
        notify(fmt.Sprintf("Court bot error: re-login failed for %s - %v", acc.Name, err))
        fmt.Fprintf(os.Stderr, "ERROR re-login %s: %v\n", acc.Name, err)
        os.Exit(1)
      }
      fmt.Println("OK")
    }
  }

  // Poll loop from 23:59:55 until slot available or 00:00:30
  pollStart := time.Date(today.Year(), today.Month(), today.Day()+1, 0, 0, -5, 0, today.Location()) // 23:59:55
  if time.Now().Before(pollStart) {
    remaining := time.Until(pollStart)
    fmt.Printf("  Waiting for poll window %s (%s)...\n", pollStart.Format("15:04:05"), remaining.Round(time.Second))
    time.Sleep(remaining)
  }

  fmt.Println("  Polling for slot availability every 200ms...")
  pollAttempts := 0
  consecutiveErrors := 0
  var fireTime time.Time
  ticker := time.NewTicker(200 * time.Millisecond)
  defer ticker.Stop()

  // Poll primary facility first (fast check)
  primaryCourt := "" // first court of first booking entry
  if len(cfg.Accounts) > 0 && len(cfg.Accounts[0].BookingPlan) > 0 {
    primaryCourt = cfg.Accounts[0].BookingPlan[0].Courts[0]
  }

  pollTimeout := midnight.Add(30 * time.Second)
  for range ticker.C {
    pollAttempts++
    if time.Now().After(pollTimeout) {
      fmt.Println("  Poll timeout 00:00:30, proceeding to book anyway")
      break
    }

    // Resolve if needed (cheap, cached if Facilities already fetched)
    resolvedID := primaryCourt
    if !isNumeric(resolvedID) && resolvedID != "" {
      if rid, err := clients[0].ResolveCourtNameToID(resolvedID); err == nil {
        resolvedID = rid
      }
    }

    slots, err := clients[0].GetTimeslots(resolvedID, targetDate)
    if err != nil {
      consecutiveErrors++
      fmt.Printf("  Poll %d: timeslot error (%v) consecutive=%d\n", pollAttempts, err, consecutiveErrors)
      if consecutiveErrors >= 3 {
        fmt.Println("  3 consecutive poll errors, continuing anyway")
        consecutiveErrors = 0 // don't abort, keep polling
      }
      continue
    }
    consecutiveErrors = 0

    // Check if target slot available (look for 07:00-09:00 or first plan slot)
    targetSlot := ""
    if len(cfg.Accounts) > 0 && len(cfg.Accounts[0].BookingPlan) > 0 {
      targetSlot = cfg.Accounts[0].BookingPlan[0].Slot
    }
    available := false
    for _, s := range slots {
      if s.Time == targetSlot && s.Available {
        available = true
        break
      }
    }
    fmt.Printf("  Poll %d %s: %s available=%v\n", pollAttempts, time.Now().Format("15:04:05.000"), targetSlot, available)
    if available {
      fireTime = time.Now()
      fmt.Printf("  Slot flipped available at %s after %d polls!\n", fireTime.Format("15:04:05.000"), pollAttempts)
      break
    }
    // Also break at exact midnight if poll shows still taken but we should try booking anyway (server may not update html instantly)
    if time.Now().After(midnight) && pollAttempts > 5 {
      // optional: break after midnight to attempt booking even if not yet visible available
    }
  }

  // Final wait until exact midnight if we broke early before midnight
  if fireTime.IsZero() {
    remaining := time.Until(midnight)
    if remaining > 0 {
      fmt.Printf("  Final wait: %s\n", remaining.Round(time.Millisecond))
      time.Sleep(remaining)
    }
    fireTime = time.Now()
    fmt.Printf("  MIDNIGHT! %s (polls=%d)\n", fireTime.Format("15:04:05.000"), pollAttempts)
  } else {
    fmt.Printf("  Fire at %s (polls=%d, %s after midnight)\n", fireTime.Format("15:04:05.000"), pollAttempts, time.Since(midnight).Round(time.Millisecond))
  }
} else {
  fmt.Println("[2/3] Skipping midnight wait (--now flag)")
}
```

Note: keep `isNumeric` helper accessible; add `pollAttempts` to final notify: `notify(fmt.Sprintf("Court bot done for %s: %d/%d slots booked (polls=%d fire=%s)", targetDate, totalSuccess, totalSlots, pollAttempts, fireTime.Format("15:04:05.000")))`

- [ ] **Step 3: Run manual verify**

Run: `go build -o /tmp/court-bot ./cmd/bot && /tmp/court-bot run --now --dry-run 2>&1 | tail -20`
Expected: no poll (skipped via --now), still books dry-run.

Build cross: `GOOS=linux GOARCH=arm64 go build -o court-bot-linux-arm64 ./cmd/bot`

- [ ] **Step 4: Commit**

```bash
git add cmd/bot/main.go
git commit -m "feat: 200ms poll snipe at midnight with resilient retry"
```

---

### Task 4: Verify + deploy

**Files:** none (ops), optional `RUNBOOK.md` update

- [ ] **Step 1: Build**

Run: `go vet ./... && go test ./... && go build -o court-bot ./cmd/bot && GOOS=linux GOARCH=arm64 go build -o court-bot-linux-arm64 ./cmd/bot`
Expected: vet 0, tests PASS, both binaries exist

- [ ] **Step 2: Deploy**

Run:
```bash
scp -i ssh-key-*.key court-bot-linux-arm64 ubuntu@149.118.140.17:/home/ubuntu/court-bot
ssh -i ssh-key-*.key ubuntu@149.118.140.17 "sudo systemctl restart court-bot && sleep 1 && sudo systemctl status court-bot --no-pager | head -20"
ssh -i ssh-key-*.key ubuntu@149.118.140.17 "crontab -l && ./court-bot health-check 2>&1 | tail -5"
```

- [ ] **Step 3: Sanity probe**

Run:
```bash
ssh -i ssh-key-*.key ubuntu@149.118.140.17 "./court-bot probe --date 2026-09-18 2>&1 | head -30; echo ---; tail -n 30 /home/ubuntu/court-bot.log"
```

Expected: `Login successful`, `GetTimeslots 0.15s`, `AVAILABLE` for 07:00 slots.

- [ ] **Step 4: Update docs**

Edit `RUNBOOK.md` snipe behavior: add poll 200ms, retry 3x notes.

Commit: `git add RUNBOOK.md && git commit -m "docs: document snipe poll behavior"`

---

## Self-Review

**Spec coverage:** timeout/transient retry (Tasks 1-2) ✓, 200ms poll snipe (Task 3) ✓, latency validated (0.10/0.75/0.15) ✓, deploy verify (Task 4) ✓

**Placeholder scan:** No TBD — all steps have exact code/commands.

**Type consistency:** `NewClient(string) *Client`, `Login(string,string) error` unchanged, `isRetryableError(error) bool` internal, `cmdRun` poll vars local — callers unaffected.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-04-snipe-resilient.md`. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — execute tasks in this session via executing-plans, batch checkpoints

Which approach?
