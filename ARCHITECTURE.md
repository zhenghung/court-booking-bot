# Architecture & Flows

CLI bot for `gpropsystems.com`. See `README.md` for usage, `RUNBOOK.md` for ops.

```mermaid
flowchart LR
    Cron["Cron / systemd"] --> CLI["court-bot CLI"]
    CLI --> Gprop["gpropsystems API"]
    CLI --> TG["Telegram API"]
    TG --> User["User group chat"]
```

Participants used below: `CLI` = `cmd/bot/main.go`, `Client` = `internal/api/client.go`, `Gprop` = gpropsystems, `TG` = Telegram Bot API, `Cron` = cron/systemd.

## 1. Login (shared sub-flow)

Used by `probe`, `book`, `run`, `facilities`, `health-check`, `/bookings`.

```mermaid
sequenceDiagram
    participant CLI
    participant Client
    participant Gprop
    CLI->>Client: Login(email, password)
    Client->>Gprop: GET /login
    Gprop-->>Client: HTML + cookies
    Client->>Client: fetchCSRFToken() parse csrf_token
    Client->>Gprop: POST /login/login_data_submit<br/>(email, password, _co6sO0rpsfat)
    Gprop-->>Client: JSON msg=Logged In Successfully<br/>+ user, ci_session cookies<br/>+ rotated CSRF token
    Client-->>CLI: OK (3x retry on timeout/5xx, 1s/2s backoff)
```

## 2. ping

```mermaid
sequenceDiagram
    participant CLI
    participant Gprop
    CLI->>Gprop: GET BaseURL
    Gprop-->>CLI: HTTP status
```

## 3. probe

```mermaid
sequenceDiagram
    participant CLI
    participant Client
    participant Gprop
    CLI->>Client: Login()
    Client->>Gprop: GET /login + POST /login/login_data_submit
    Gprop-->>Client: session
    loop each court in FacilityIDs
        CLI->>Client: ResolveCourtNameToID(name or ID)
        Client->>Gprop: GET /booking/add_new_booking (if name)
        Gprop-->>Client: HTML option list
        CLI->>Client: GetTimeslots(facilityID, date)
        Client->>Gprop: POST /booking/get_booking_timeslot
        Gprop-->>Client: JSON html timeslot blocks
        Client->>Client: parseTimeslotHTML() btn-grey vs taken
    end
    Client-->>CLI: print AVAILABLE / TAKEN per court
```

## 4. book

```mermaid
sequenceDiagram
    participant CLI
    participant Client
    participant Gprop
    CLI->>Client: Login()
    CLI->>Client: ResolveCourtNameToID() per court
    loop courts in priority order
        CLI->>Client: GetTimeslots(facilityID, date)
        Client->>Gprop: POST /booking/get_booking_timeslot
        Gprop-->>Client: slots
    end
    Note over CLI: pick first court where slot Time == Available
    alt dry-run
        CLI-->>CLI: print Would book, exit
    else live
        CLI->>Client: BookSlot(facilityID, unitID, name, contact, date, slot)
        Client->>Gprop: POST /booking/add_new_booking_action<br/>(multipart otherData + CSRF)
        Gprop-->>Client: JSON status, msg, insertID
    end
```

## 5. run (midnight snipe)

Cron: `0 0 * * 5 ./court-bot run --now`. Target date = today+7d. `--now` skips wait.

```mermaid
sequenceDiagram
    participant Cron
    participant CLI
    participant Client
    participant Gprop
    participant TG
    Cron->>CLI: run
    CLI->>CLI: targetDate = today+7d<br/>check Weekday == TargetDay
    alt wrong weekday and no --now
        CLI->>TG: skipped notification, exit
    else proceed
        loop each Account
            CLI->>Client: Login()
            Client->>Gprop: login flow
        end
        alt wait enabled (no --now)
            CLI->>CLI: sleep until T-30s, re-login all accounts
            CLI->>CLI: sleep until 23:59:55
            loop poll every 200ms until 00:00:30
                CLI->>Client: GetTimeslots(primaryCourt, targetDate)
                Client->>Gprop: POST get_booking_timeslot
                Gprop-->>Client: slots
            end
            Note over CLI: clamp early flip to midnight,<br/>never book before 00:00:00
        end
        loop each Account / each BookingPlan entry
            CLI->>Client: ResolveCourtNameToID()
            loop courts in priority, 3x retry 1s/2s
                CLI->>Client: BookSlot()
                Client->>Gprop: POST add_new_booking_action
                Gprop-->>Client: SUCCESS or REJECTED
            end
        end
        CLI->>TG: done dry-run or booked x/y (polls, fire, delay)
    end
```

## 6. bot daemon (Telegram)

Polls `getUpdates` every 2s. Filters by `ChatID`. Commands: `/status`, `/setday`, `/bookings`, `/help`.

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant TG
    participant Gprop
    loop every 2s
        CLI->>TG: GET getUpdates offset
        TG-->>CLI: updates
    end
    User->>TG: /status | /setday | /bookings | /help
    TG-->>CLI: message
    alt /status
        CLI->>CLI: Load config, compute next run
        CLI->>TG: sendMessage status + booking plan
    else /setday monday
        CLI->>CLI: setEnvKey ~/.env GPROP_TARGET_DAY
        CLI->>CLI: crontab -l rewrite scheduler line, crontab -
        CLI->>TG: sendMessage updated day + cron line
    else /bookings
        CLI->>Gprop: Login per account + GET booking_listing
        Gprop-->>CLI: aaData bookings
        CLI->>TG: sendMessage upcoming bookings
    else /help
        CLI->>TG: sendMessage command list
    end
```

## 7. facilities

```mermaid
sequenceDiagram
    participant CLI
    participant Client
    participant Gprop
    CLI->>Client: Login()
    CLI->>Client: GetFacilities()
    Client->>Gprop: GET /booking/add_new_booking
    Gprop-->>Client: HTML
    Client->>Client: parseFacilityHTML() option value=ID
    Client-->>CLI: ID + Name table
```

## 8. health-check

Cron daily `0 8 * * *`. Alerts only on failure.

```mermaid
sequenceDiagram
    participant Cron
    participant CLI
    participant Client
    participant Gprop
    participant TG
    Cron->>CLI: health-check
    CLI->>Client: Login()
    Client->>Gprop: login flow
    alt login OK
        CLI-->>CLI: Health check passed, exit 0, no Telegram
    else login FAIL
        CLI->>TG: sendMessage Health Check Failed + error + time
        CLI-->>CLI: exit 1
    end
```
