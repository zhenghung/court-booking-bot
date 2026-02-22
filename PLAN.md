# Court Booking Bot - Implementation Plan

## Overview
Automated bot to book court slots on gpropsystems exactly when they become available (midnight, 7 days in advance).

## Target Configuration
- **Booking site**: gpropsystems (https://www.gpropsystems.com)
- **Authentication**: Username + Password
- **Default slot**: Friday mornings - 7:00-8:00 AM AND 8:00-9:00 AM (configurable)
- **Hosting**: Oracle Cloud Free Tier (always-free VM)
- **Language**: Go
- **Notifications**: Telegram (deferred - add later)

---

## Strategy: API-First with Browser Fallback

### Primary Approach: Direct API Calls (Preferred)
- Analyze gpropsystems network traffic to discover API endpoints
- Replicate the login + booking flow via HTTP requests
- Benefits: Fast (~100ms), reliable, lightweight, no browser needed

### Fallback: Browser Automation
- Only if API approach fails (e.g., heavy anti-bot protection)
- Use Go + `rod` library for browser control

---

## Architecture (API Approach)

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│  Cron Job   │───▶│   Go Binary  │───▶│ gpropsystems│
│ (midnight)  │    │  (HTTP API)  │    │     API     │
└─────────────┘    └──────────────┘    └─────────────┘
                          │
                          ▼
                   ┌──────────────┐
                   │   Telegram   │
                   │ Notification │
                   └──────────────┘
```

---

## API Discovery (Discovered)

### Authentication
**Login Page**: `https://www.gpropsystems.com/login`

**Login Endpoint**: `POST https://www.gpropsystems.com/login/login_data_submit`
- Content-Type: `application/x-www-form-urlencoded`
- Fields:
  - `email` - User email
  - `password` - User password
  - `_co6sO0rpsfat` - CSRF token (must fetch from login page first)
- Returns: JSON with user properties, roles, and sets session cookies

**Cookies Set After Login**:
- `user` - Encrypted user session token
- `_cmhsYhr8mfXz` - CSRF token cookie
- `ci_session` - CodeIgniter session ID

### Get Available Timeslots
**Endpoint**: `POST https://www.gpropsystems.com/booking/get_booking_timeslot`
- Content-Type: `application/x-www-form-urlencoded`
- Headers:
  - `X-Requested-With: XMLHttpRequest`
- Fields:
  - `is_ajax=1`
  - `facilityId` - The court/facility ID (e.g., `7938`)
  - `bookingDate` - Target date in `YYYY-MM-DD` format
  - `_co6sO0rpsfat` - CSRF token
- Cookies Required: `user`, `_cmhsYhr8mfXz`, `ci_session`
- Returns: JSON with available timeslots (need sample response)

### Create Booking
**Endpoint**: TBD - need to capture from browser
- Likely: `POST https://www.gpropsystems.com/booking/...`

---

## Implementation Steps

### Phase 1: API Discovery
1. ✅ Login endpoint discovered
2. ✅ Get timeslots endpoint discovered
3. ⏳ **TODO**: Capture create booking endpoint (need cURL from user)

### Phase 2: Project Setup
```
court-booking-bot/
├── cmd/
│   └── bot/
│       └── main.go          # Entry point
├── internal/
│   ├── api/
│   │   └── client.go        # HTTP client for gpropsystems
│   ├── booking/
│   │   └── booking.go       # Booking logic
│   ├── config/
│   │   └── config.go        # Configuration management
│   └── notify/
│       └── telegram.go      # Telegram notifications
├── go.mod
├── go.sum
├── .env.example
├── .gitignore
└── README.md
```

### Phase 3: Core Implementation
1. **API Client** (`internal/api/client.go`)
   - HTTP client with cookie jar for session management
   - Login function (POST credentials, store session)
   - GetAvailableSlots function
   - CreateBooking function
   - Proper error handling and retries

2. **Booking Logic** (`internal/booking/booking.go`)
   - Accept any date/time/court as input (for flexible testing)
   - Execute booking with retry logic
   - Handle "slot taken" gracefully
   - Dry-run mode to test without actual booking

3. **CLI Interface** (`cmd/bot/main.go`)
   - `book` command with flags: `--date`, `--time`, `--court`, `--dry-run`
   - Config file support
   - Environment variable override

4. **Telegram Notifications** (`internal/notify/telegram.go`) - DEFERRED
   - Add later once core booking works

### Phase 4: Configuration (Fully Flexible for Testing)

**CLI flags** for easy testing:
```bash
# Test booking for any date/time/court
./court-bot book \
  --date 2025-01-07 \
  --time 09:00 \
  --court 2 \
  --dry-run          # Optional: simulate without actual booking

# Or use config file / env vars for production
./court-bot book --config config.yaml
```

**Config file** (`config.yaml`):
```yaml
gprop:
  base_url: "https://..."
  username: "your_username"
  password: "your_password"

booking:
  target_day: "friday"       # For auto-scheduling
  target_slots:
    - "07:00"
    - "08:00"
  court_preference: 1

# Optional - add later
telegram:
  enabled: false
  bot_token: ""
  chat_id: ""
```

**Environment variables** (override config):
```
GPROP_BASE_URL=https://...
GPROP_USERNAME=your_username
GPROP_PASSWORD=your_password
```

### Phase 5: Deployment on Oracle Cloud Free Tier
1. Create always-free ARM instance (4 OCPU, 24GB RAM)
2. Build Go binary: `GOOS=linux GOARCH=arm64 go build`
3. Copy binary to server via SCP
4. Set up cron:
   ```bash
   # Run at 00:00:01 every Friday
   1 0 * * 5 /home/user/court-bot >> /var/log/court-bot.log 2>&1
   ```

---

## Key Technical Considerations

### Why Go for API Calls?
- Single binary deployment (no dependencies)
- Excellent HTTP client (net/http)
- Low memory footprint (~5-10MB)
- Fast startup time
- Strong typing prevents runtime errors

### Timing Strategy
- Use NTP sync on server for accurate time
- Start execution at 23:59:55
- Pre-authenticate and prepare request
- Fire booking request at exactly 00:00:00
- Use `time.Until()` for precise timing

### Error Handling & Retry
- Retry up to 3 times on network failure
- Exponential backoff (1s, 2s, 4s)
- Different handling for "slot taken" vs "error"
- Log all attempts for debugging

### Security
- Store credentials in environment variables
- Never commit `.env` to git
- Use HTTPS for all requests

---

## Current Progress
- [x] Login URL and endpoint discovered
- [x] Get timeslots endpoint discovered
- [ ] **NEEDED**: Create booking endpoint (cURL from user)
- [ ] **NEEDED**: Sample timeslot response JSON
- [ ] **NEEDED**: Preferred court/facility ID

---

## Next Steps
1. ⏳ **Get create booking cURL** - User to capture when making a booking
2. **Build API client**: Implement login + booking HTTP calls in Go
3. **Add CLI**: Implement flexible command-line interface
4. **Local testing**: Test with any date/time/court
5. **Deploy**: Set up Oracle Cloud Free Tier instance
6. **Add notifications**: (Later) Telegram integration
7. **Production test**: Run a live booking
