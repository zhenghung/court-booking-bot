package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhenghung/court-booking-bot/internal/api"
	"github.com/zhenghung/court-booking-bot/internal/config"
)

func main() {
	fmt.Println("=== Court Booking Bot ===")
	fmt.Println()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "ping":
		cmdPing()
	case "probe":
		cmdProbe()
	case "book":
		cmdBook()
	case "run":
		cmdRun()
	case "bot":
		cmdBot()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: court-bot <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  ping     Test HTTP connectivity to gpropsystems")
	fmt.Println("  probe    Login and fetch timeslots for a given date")
	fmt.Println("  book     Book a specific timeslot (tries all courts)")
	fmt.Println("  run      Scheduler: wait for midnight and auto-book target slots")
	fmt.Println("  bot      Run Telegram bot daemon (listens for /status and /setday)")
	fmt.Println()
	fmt.Println("Run 'court-bot <command> --help' for command flags.")
}

func cmdPing() {
	cfg, err := config.Load()
	if err != nil {
		// For ping, we don't need credentials — just use the default URL
		cfg = &config.Config{BaseURL: "https://www.gpropsystems.com"}
	}

	client := api.NewClient(cfg.BaseURL)
	status, err := client.Ping()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("GET %s -> %s\n", cfg.BaseURL, status)
	fmt.Println("Connectivity OK!")
}

func cmdProbe() {
	probeFlags := flag.NewFlagSet("probe", flag.ExitOnError)
	date := probeFlags.String("date", "", "Target date in YYYY-MM-DD format (default: 7 days from now)")
	facilityID := probeFlags.String("facility", "", "Facility ID (overrides .env)")
	probeFlags.Parse(os.Args[2:])

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	facilityIDs := cfg.FacilityIDs
	if *facilityID != "" {
		facilityIDs = []string{*facilityID}
	}

	targetDate := *date
	if targetDate == "" {
		targetDate = time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	}

	fmt.Printf("Courts:   %v\n", facilityIDs)
	fmt.Printf("Date:     %s\n", targetDate)
	fmt.Println()

	// Step 1: Login
	fmt.Println("[1/2] Logging in...")
	client := api.NewClient(cfg.BaseURL)
	if err := client.Login(cfg.Email, cfg.Password); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Login successful!")
	fmt.Println()

	// Step 2: Fetch timeslots for each court
	fmt.Println("[2/2] Fetching timeslots...")
	for _, fid := range facilityIDs {
		fmt.Printf("\n  Court %s:\n", fid)
		slots, err := client.GetTimeslots(fid, targetDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "    ERROR: %v\n", err)
			continue
		}

		for _, slot := range slots {
			status := "AVAILABLE"
			if !slot.Available {
				status = "TAKEN"
			}
			fmt.Printf("    %s  [%s]\n", slot.Time, status)
		}
	}
}

func cmdBook() {
	bookFlags := flag.NewFlagSet("book", flag.ExitOnError)
	date := bookFlags.String("date", "", "Target date in YYYY-MM-DD format (default: 7 days from now)")
	timeSlot := bookFlags.String("time", "", "Time slot to book, e.g. 07:00-08:00 (required)")
	facilityID := bookFlags.String("facility", "", "Facility ID (overrides .env, tries single court)")
	dryRun := bookFlags.Bool("dry-run", false, "Check availability without actually booking")
	bookFlags.Parse(os.Args[2:])

	if *timeSlot == "" {
		fmt.Fprintf(os.Stderr, "ERROR: --time is required (e.g. --time 07:00-08:00)\n")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	facilityIDs := cfg.FacilityIDs
	if *facilityID != "" {
		facilityIDs = []string{*facilityID}
	}

	targetDate := *date
	if targetDate == "" {
		targetDate = time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	}

	fmt.Printf("Courts:   %v\n", facilityIDs)
	fmt.Printf("Date:     %s\n", targetDate)
	fmt.Printf("Time:     %s\n", *timeSlot)
	if *dryRun {
		fmt.Println("Mode:     DRY RUN (no booking will be made)")
	}
	fmt.Println()

	// Step 1: Login
	fmt.Println("[1/3] Logging in...")
	client := api.NewClient(cfg.BaseURL)
	if err := client.Login(cfg.Email, cfg.Password); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Login successful!")
	fmt.Println()

	// Step 2: Find an available court for the requested slot
	fmt.Println("[2/3] Checking availability...")
	var availableCourt string
	for _, fid := range facilityIDs {
		slots, err := client.GetTimeslots(fid, targetDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Court %s: ERROR %v\n", fid, err)
			continue
		}
		for _, slot := range slots {
			if slot.Time == *timeSlot {
				if slot.Available {
					fmt.Printf("  Court %s: %s is AVAILABLE ✓\n", fid, *timeSlot)
					availableCourt = fid
				} else {
					fmt.Printf("  Court %s: %s is TAKEN\n", fid, *timeSlot)
				}
				break
			}
		}
		if availableCourt != "" {
			break
		}
	}

	if availableCourt == "" {
		fmt.Fprintf(os.Stderr, "\nERROR: %s is not available on any court for %s\n", *timeSlot, targetDate)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Printf("\n[DRY RUN] Would book court %s, %s on %s\n", availableCourt, *timeSlot, targetDate)
		return
	}

	// Step 3: Book the slot
	fmt.Println()
	fmt.Printf("[3/3] Booking court %s, %s on %s...\n", availableCourt, *timeSlot, targetDate)
	result, err := client.BookSlot(availableCourt, cfg.UnitID, cfg.BookingName, cfg.Contact, targetDate, *timeSlot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	if result.Status {
		fmt.Printf("  SUCCESS: %s (ID: %d)\n", result.Msg, result.InsertID)
	} else {
		fmt.Printf("  FAILED: %s - %s\n", result.MsgTitle, result.Msg)
		os.Exit(1)
	}
}

func cmdRun() {
	runFlags := flag.NewFlagSet("run", flag.ExitOnError)
	now := runFlags.Bool("now", false, "Skip midnight wait — book immediately (for testing)")
	dryRun := runFlags.Bool("dry-run", false, "Check availability without actually booking")
	runFlags.Parse(os.Args[2:])

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	// Validate scheduler config
	if cfg.TargetDay == "" {
		fmt.Fprintf(os.Stderr, "ERROR: GPROP_TARGET_DAY must be set (e.g. friday)\n")
		os.Exit(1)
	}
	targetDayOfWeek, err := parseDayOfWeek(cfg.TargetDay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	if len(cfg.BookingPlan) == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: GPROP_BOOKING_PLAN must be set (e.g. 07:00-08:00>7937,7935;08:00-09:00>7937,7936,7935)\n")
		os.Exit(1)
	}
	notifyEnabled := cfg.TelegramBotToken != "" && cfg.TelegramChatID != ""
	notify := func(msg string) {
		if !notifyEnabled {
			return
		}
		if err := sendTelegramMessage(cfg.TelegramBotToken, cfg.TelegramChatID, msg); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: failed to send Telegram notification: %v\n", err)
		}
	}

	// Calculate target date: today + 7 days (the date that opens at next midnight)
	today := time.Now()
	targetDate := today.AddDate(0, 0, 7).Format("2006-01-02")
	targetDateParsed, _ := time.Parse("2006-01-02", targetDate)

	fmt.Printf("Target day:   %s\n", cfg.TargetDay)
	fmt.Printf("Target date:  %s (%s)\n", targetDate, targetDateParsed.Weekday())
	fmt.Println("Booking plan:")
	for _, entry := range cfg.BookingPlan {
		fmt.Printf("  %s → courts %v\n", entry.Slot, entry.Courts)
	}
	if *dryRun {
		fmt.Println("Mode:         DRY RUN")
	}
	fmt.Println()

	// Check if the target date falls on the desired day of week
	if targetDateParsed.Weekday() != targetDayOfWeek {
		fmt.Printf("Nothing to book tonight — %s is a %s, not %s.\n",
			targetDate, targetDateParsed.Weekday(), targetDayOfWeek)
		fmt.Println("Run with --now to override and book anyway.")
		if !*now {
			notify(fmt.Sprintf("Court bot skipped: target date %s is %s (expected %s)", targetDate, targetDateParsed.Weekday(), targetDayOfWeek))
			return
		}
		fmt.Println("--now flag set, proceeding anyway...")
		fmt.Println()
	}

	// Step 1: Pre-authenticate
	fmt.Println("[1/3] Pre-authenticating...")
	client := api.NewClient(cfg.BaseURL)
	if err := client.Login(cfg.Email, cfg.Password); err != nil {
		notify(fmt.Sprintf("Court bot error: login failed - %v", err))
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Login successful!")
	fmt.Println()

	// Step 2: Wait for midnight (unless --now)
	if !*now {
		midnight := time.Date(today.Year(), today.Month(), today.Day()+1, 0, 0, 0, 0, today.Location())
		waitDuration := time.Until(midnight)
		fmt.Printf("[2/3] Waiting for midnight (%s)...\n", midnight.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Time until midnight: %s\n", waitDuration.Round(time.Second))

		// If more than 60s away, re-login closer to midnight to keep session fresh
		if waitDuration > 60*time.Second {
			preLoginWait := waitDuration - 30*time.Second
			fmt.Printf("  Sleeping %s, then re-authenticating...\n", preLoginWait.Round(time.Second))
			time.Sleep(preLoginWait)

			fmt.Println("  Re-authenticating (session refresh)...")
			if err := client.Login(cfg.Email, cfg.Password); err != nil {
				notify(fmt.Sprintf("Court bot error: re-login failed - %v", err))
				fmt.Fprintf(os.Stderr, "ERROR re-login: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("  Re-login successful!")
		}

		// Final wait until exact midnight
		remaining := time.Until(midnight)
		if remaining > 0 {
			fmt.Printf("  Final wait: %s\n", remaining.Round(time.Millisecond))
			time.Sleep(remaining)
		}
		fmt.Printf("  MIDNIGHT! %s\n", time.Now().Format("15:04:05.000"))
	} else {
		fmt.Println("[2/3] Skipping midnight wait (--now flag)")
	}
	fmt.Println()

	// Step 3: Book each target slot using per-slot court priority
	fmt.Println("[3/3] Booking target slots...")
	successCount := 0
	for _, entry := range cfg.BookingPlan {
		fmt.Printf("\n  === Slot: %s (courts: %v) ===\n", entry.Slot, entry.Courts)
		booked := false

		for _, fid := range entry.Courts {
			if *dryRun {
				fmt.Printf("  [DRY RUN] Would try court %s for %s on %s\n", fid, entry.Slot, targetDate)
				continue
			}

			// Try booking with retries
			var lastErr error
			for attempt := 1; attempt <= 3; attempt++ {
				if attempt > 1 {
					backoff := time.Duration(1<<(attempt-2)) * time.Second
					fmt.Printf("  Retry %d after %s...\n", attempt, backoff)
					time.Sleep(backoff)
				}

				result, err := client.BookSlot(fid, cfg.UnitID, cfg.BookingName, cfg.Contact, targetDate, entry.Slot)
				if err != nil {
					lastErr = err
					fmt.Printf("  Court %s attempt %d: ERROR %v\n", fid, attempt, err)
					continue
				}

				if result.Status {
					fmt.Printf("  Court %s: SUCCESS — %s (ID: %d)\n", fid, result.Msg, result.InsertID)
					booked = true
					successCount++
					break
				} else {
					fmt.Printf("  Court %s: REJECTED — %s\n", fid, result.Msg)
					lastErr = fmt.Errorf("%s", result.Msg)
					break // Don't retry on server rejection, try next court
				}
			}

			if booked {
				break // Move to next slot
			}
			if lastErr != nil {
				fmt.Printf("  Court %s: failed — %v\n", fid, lastErr)
			}
		}

		if !booked && !*dryRun {
			fmt.Printf("  FAILED to book %s on any court\n", entry.Slot)
		}
	}

	fmt.Println()
	if *dryRun {
		fmt.Println("=== DRY RUN complete ===")
		notify(fmt.Sprintf("Court bot dry run complete for %s (%d plan entries)", targetDate, len(cfg.BookingPlan)))
	} else {
		fmt.Printf("=== Done: %d/%d slots booked ===\n", successCount, len(cfg.BookingPlan))
		notify(fmt.Sprintf("Court bot done for %s: %d/%d slots booked", targetDate, successCount, len(cfg.BookingPlan)))
	}
}

func sendTelegramMessage(botToken, chatID, message string) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	resp, err := http.PostForm(endpoint, url.Values{
		"chat_id": {chatID},
		"text":    {message},
	})
	if err != nil {
		return fmt.Errorf("telegram request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram returned status %d", resp.StatusCode)
	}

	return nil
}

func cmdBot() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	if cfg.TelegramBotToken == "" || cfg.TelegramChatID == "" {
		fmt.Fprintf(os.Stderr, "ERROR: GPROP_TELEGRAM_BOT_TOKEN and GPROP_TELEGRAM_CHAT_ID must be set\n")
		os.Exit(1)
	}

	fmt.Println("Starting Telegram bot daemon...")
	fmt.Printf("  Chat ID: %s\n", cfg.TelegramChatID)
	fmt.Println("  Listening for /status, /setday, /bookings commands...")
	fmt.Println()

	var lastUpdateID int64 = 0

	for {
		updates, err := getTelegramUpdates(cfg.TelegramBotToken, lastUpdateID+1)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: failed to get updates: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			lastUpdateID = update.UpdateID

			if update.Message == nil {
				continue
			}

			chatIDStr := fmt.Sprintf("%d", update.Message.Chat.ID)
			if chatIDStr != cfg.TelegramChatID {
				continue
			}

			text := strings.TrimSpace(update.Message.Text)
			cmd, arg := parseBotCommand(text)
			switch cmd {
			case "/status":
				currentCfg, err := config.Load()
				if err != nil {
					_ = sendTelegramMessage(cfg.TelegramBotToken, cfg.TelegramChatID,
						fmt.Sprintf("⚠️ Failed to load config: %v", err))
					continue
				}
				fmt.Printf("[%s] Received /status from %s\n",
					time.Now().Format("15:04:05"), update.Message.From.Username)
				handleStatusCommand(currentCfg)
			case "/setday":
				fmt.Printf("[%s] Received /setday from %s\n",
					time.Now().Format("15:04:05"), update.Message.From.Username)
				handleSetDayCommand(cfg.TelegramBotToken, cfg.TelegramChatID, arg)
			case "/bookings":
				fmt.Printf("[%s] Received /bookings from %s\n",
					time.Now().Format("15:04:05"), update.Message.From.Username)
				handleBookingsCommand(cfg)
			case "/help":
				fmt.Printf("[%s] Received /help from %s\n",
					time.Now().Format("15:04:05"), update.Message.From.Username)
				handleHelpCommand(cfg.TelegramBotToken, cfg.TelegramChatID)
			}
		}

		time.Sleep(2 * time.Second)
	}
}

func getTelegramUpdates(botToken string, offset int64) ([]TelegramUpdate, error) {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", botToken, offset)
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result TelegramUpdatesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if !result.OK {
		return nil, fmt.Errorf("telegram API error")
	}

	return result.Result, nil
}

type TelegramUpdatesResponse struct {
	OK     bool             `json:"ok"`
	Result []TelegramUpdate `json:"result"`
}

type TelegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *TelegramMessage `json:"message"`
}

type TelegramMessage struct {
	Text string       `json:"text"`
	Chat TelegramChat `json:"chat"`
	From TelegramUser `json:"from"`
}

type TelegramChat struct {
	ID int64 `json:"id"`
}

type TelegramUser struct {
	Username string `json:"username"`
}

func handleStatusCommand(cfg *config.Config) {
	targetDay, _ := parseDayOfWeek(cfg.TargetDay)
	today := time.Now()
	targetDate := today.AddDate(0, 0, 7).Format("2006-01-02")
	targetDateParsed, _ := time.Parse("2006-01-02", targetDate)

	nextFriday := today
	for nextFriday.Weekday() != targetDay {
		nextFriday = nextFriday.AddDate(0, 0, 1)
	}
	nextRun := time.Date(nextFriday.Year(), nextFriday.Month(), nextFriday.Day(), 0, 0, 0, 0, today.Location())
	if nextRun.Before(today) {
		nextRun = nextRun.AddDate(0, 0, 7)
	}

	var planStr string
	for _, entry := range cfg.BookingPlan {
		planStr += fmt.Sprintf("\n  • %s → courts %v", entry.Slot, entry.Courts)
	}

	status := fmt.Sprintf(`🥒🎾 Court Bot Status

📅 Target day: %s
📆 Next booking date: %s (%s)
⏰ Next cron run: %s
⏱ Time until run: %s

📋 Booking plan:%s

✅ Bot is running`,
		cfg.TargetDay,
		targetDate,
		targetDateParsed.Weekday(),
		nextRun.Format("Mon Jan 2, 15:04"),
		time.Until(nextRun).Round(time.Minute),
		planStr,
	)

	if err := sendTelegramMessage(cfg.TelegramBotToken, cfg.TelegramChatID, status); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to send status: %v\n", err)
	}
}

func handleSetDayCommand(botToken, chatID, dayInput string) {
	if strings.TrimSpace(dayInput) == "" {
		_ = sendTelegramMessage(botToken, chatID,
			"Usage: /setday <day>\nExample: /setday monday\nAllowed: sunday, monday, tuesday, wednesday, thursday, friday, saturday")
		return
	}

	day, err := normalizeDayInput(dayInput)
	if err != nil {
		_ = sendTelegramMessage(botToken, chatID,
			fmt.Sprintf("❌ Invalid day: %q\nUse: sunday, monday, tuesday, wednesday, thursday, friday, saturday", dayInput))
		return
	}

	weekday, _ := parseDayOfWeek(day)
	envPath := envFilePath()
	originalEnv, err := setEnvKey(envPath, "GPROP_TARGET_DAY", day)
	if err != nil {
		_ = sendTelegramMessage(botToken, chatID, fmt.Sprintf("❌ Failed to update %s: %v", envPath, err))
		return
	}

	cronLine, err := updateSchedulerCron(weekday)
	if err != nil {
		rollbackErr := os.WriteFile(envPath, originalEnv, 0o600)
		if rollbackErr != nil {
			_ = sendTelegramMessage(botToken, chatID,
				fmt.Sprintf("❌ Failed to update cron: %v\n⚠️ Also failed to roll back %s: %v", err, envPath, rollbackErr))
			return
		}
		_ = sendTelegramMessage(botToken, chatID,
			fmt.Sprintf("❌ Failed to update cron: %v\nℹ️ .env change was rolled back.", err))
		return
	}

	if err := os.Setenv("GPROP_TARGET_DAY", day); err != nil {
		_ = sendTelegramMessage(botToken, chatID,
			fmt.Sprintf("⚠️ Day updated, but failed to refresh runtime env: %v", err))
		return
	}

	_ = sendTelegramMessage(botToken, chatID,
		fmt.Sprintf("✅ Booking day updated to %s\n🕛 Cron: %s", day, cronLine))
}

func handleBookingsCommand(cfg *config.Config) {
	client := api.NewClient(cfg.BaseURL)
	if err := client.Login(cfg.Email, cfg.Password); err != nil {
		_ = sendTelegramMessage(cfg.TelegramBotToken, cfg.TelegramChatID,
			fmt.Sprintf("❌ Login failed: %v", err))
		return
	}

	bookings, err := client.GetBookings()
	if err != nil {
		_ = sendTelegramMessage(cfg.TelegramBotToken, cfg.TelegramChatID,
			fmt.Sprintf("❌ Failed to fetch bookings: %v", err))
		return
	}

	if len(bookings) == 0 {
		_ = sendTelegramMessage(cfg.TelegramBotToken, cfg.TelegramChatID,
			"📋 No upcoming bookings found.")
		return
	}

	msg := "🥒🎾 Upcoming Bookings\n\n"
	for _, b := range bookings {
		// Format time: "07:00:00" -> "07:00"
		timeStart := b.TimeStart
		if len(timeStart) >= 5 {
			timeStart = timeStart[:5]
		}
		timeEnd := b.TimeEnd
		if len(timeEnd) >= 5 {
			timeEnd = timeEnd[:5]
		}

		// Format date: "2026-04-24" -> "Apr 24 (Fri)"
		dateStr := b.Date
		if t, err := time.Parse("2006-01-02", b.Date); err == nil {
			dateStr = t.Format("Jan 2 (Mon)")
		}

		statusEmoji := "✅"
		if b.Status == "Pending" {
			statusEmoji = "⏳"
		} else if b.Status == "Cancelled" || b.Status == "Rejected" {
			statusEmoji = "❌"
		}

		msg += fmt.Sprintf("%s %s\n   📍 %s\n   🕐 %s - %s\n\n",
			statusEmoji, dateStr, b.Facility, timeStart, timeEnd)
	}

	_ = sendTelegramMessage(cfg.TelegramBotToken, cfg.TelegramChatID, msg)
}

func handleHelpCommand(botToken, chatID string) {
	help := `🥒🎾 Court Bot Commands

/status — Check bot config and next scheduled run
/setday <day> — Change booking day (e.g. /setday monday)
/bookings — Show upcoming bookings
/help — Show this help message`

	_ = sendTelegramMessage(botToken, chatID, help)
}

func parseBotCommand(text string) (string, string) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", ""
	}

	cmd := strings.ToLower(strings.SplitN(fields[0], "@", 2)[0])
	arg := ""
	if len(fields) > 1 {
		arg = strings.ToLower(strings.TrimSpace(fields[1]))
	}
	return cmd, arg
}

func normalizeDayInput(s string) (string, error) {
	day, err := parseDayOfWeek(strings.ToLower(strings.TrimSpace(s)))
	if err != nil {
		return "", err
	}
	return strings.ToLower(day.String()), nil
}

func envFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".env"
	}
	return filepath.Join(home, ".env")
}

func setEnvKey(path, key, value string) ([]byte, error) {
	original, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := strings.ReplaceAll(string(original), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	prefix := key + "="
	found := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			lines[i] = prefix + value
			found = true
		}
	}

	if !found {
		lines = append(lines, prefix+value)
	}

	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}

	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return nil, err
	}

	return original, nil
}

func weekdayToCronNumber(day time.Weekday) int {
	return int(day)
}

func updateSchedulerCron(day time.Weekday) (string, error) {
	const schedulerCmd = "cd /home/ubuntu && ./court-bot run --now >> /home/ubuntu/court-bot.log 2>&1"
	newLine := fmt.Sprintf("0 0 * * %d %s", weekdayToCronNumber(day), schedulerCmd)

	listCmd := exec.Command("crontab", "-l")
	listOut, listErr := listCmd.CombinedOutput()

	existing := ""
	if listErr == nil {
		existing = string(listOut)
	} else {
		stderr := strings.ToLower(string(listOut))
		if !strings.Contains(stderr, "no crontab for") {
			return "", fmt.Errorf("failed to read crontab: %w (%s)", listErr, strings.TrimSpace(string(listOut)))
		}
	}

	var buffer bytes.Buffer
	for _, line := range strings.Split(existing, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(line, schedulerCmd) {
			continue
		}
		buffer.WriteString(line)
		buffer.WriteString("\n")
	}
	buffer.WriteString(newLine)
	buffer.WriteString("\n")

	setCmd := exec.Command("crontab", "-")
	setCmd.Stdin = strings.NewReader(buffer.String())
	setOut, setErr := setCmd.CombinedOutput()
	if setErr != nil {
		return "", fmt.Errorf("failed to write crontab: %w (%s)", setErr, strings.TrimSpace(string(setOut)))
	}

	return newLine, nil
}

func parseDayOfWeek(s string) (time.Weekday, error) {
	days := map[string]time.Weekday{
		"sunday": time.Sunday, "sun": time.Sunday,
		"monday": time.Monday, "mon": time.Monday,
		"tuesday": time.Tuesday, "tue": time.Tuesday,
		"wednesday": time.Wednesday, "wed": time.Wednesday,
		"thursday": time.Thursday, "thu": time.Thursday,
		"friday": time.Friday, "fri": time.Friday,
		"saturday": time.Saturday, "sat": time.Saturday,
	}
	day, ok := days[strings.ToLower(s)]
	if !ok {
		return 0, fmt.Errorf("invalid day of week: %q (use monday, tuesday, etc.)", s)
	}
	return day, nil
}
