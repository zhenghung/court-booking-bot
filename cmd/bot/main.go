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
	"github.com/zhenghung/court-booking-bot/internal/web"
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
	case "facilities":
		cmdFacilities()
	case "health-check":
		cmdHealthCheck()
	case "serve":
		cmdServe()
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
	fmt.Println("  ping         Test HTTP connectivity to gpropsystems")
	fmt.Println("  probe        Login and fetch timeslots for a given date")
	fmt.Println("  book         Book a specific timeslot (tries all courts)")
	fmt.Println("  run          Scheduler: wait for midnight and auto-book target slots")
	fmt.Println("  bot          Run Telegram bot daemon (listens for /status and /setday)")
	fmt.Println("  facilities   List all available courts with their IDs and names")
	fmt.Println("  health-check Test login functionality and alert on failure")
	fmt.Println("  serve        Run embedded web UI (JSON API + SPA)")
	fmt.Println()
	fmt.Println("Run 'court-bot <command> --help' for command flags.")
}

// scheduleFlag collects repeatable --schedule values.
type scheduleFlag []string

func (s *scheduleFlag) String() string { return strings.Join(*s, ",") }
func (s *scheduleFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// scheduleFileLabel reports where schedules came from for error messages.
func scheduleFileLabel(cfg *config.Config) string {
	if cfg.ScheduleFile != "" {
		return cfg.ScheduleFile
	}
	return "no file (set GPROP_SCHEDULES_FILE or ./schedules.yaml)"
}

// primaryCredentials returns login creds + default courts.
// Prefers legacy single-account env; falls back to Accounts[0] so
// probe/book/facilities work under multi-account config.
func primaryCredentials(cfg *config.Config) (email, password string, courts []string) {
	if cfg.Email != "" && cfg.Password != "" {
		return cfg.Email, cfg.Password, cfg.FacilityIDs
	}
	if len(cfg.Accounts) > 0 {
		var union []string
		seen := map[string]bool{}
		for _, acc := range cfg.Accounts {
			for _, e := range acc.BookingPlan {
				for _, c := range e.Courts {
					if !seen[c] {
						seen[c] = true
						union = append(union, c)
					}
				}
			}
		}
		courts = cfg.FacilityIDs
		if len(courts) == 0 {
			courts = union
		}
		return cfg.Accounts[0].Email, cfg.Accounts[0].Password, courts
	}
	return "", "", cfg.FacilityIDs
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
	scheduleName := probeFlags.String("schedule", "", "Schedule name from schedules.yaml (uses its courts + account)")
	probeFlags.Parse(os.Args[2:])

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	email, password, defaultCourts := primaryCredentials(cfg)
	facilityIDs := defaultCourts
	if *facilityID != "" {
		facilityIDs = []string{*facilityID}
	}
	if *scheduleName != "" {
		if len(cfg.Schedules) == 0 {
			fmt.Fprintf(os.Stderr, "ERROR: --schedule %q given but no schedules file loaded (%s)\n", *scheduleName, scheduleFileLabel(cfg))
			os.Exit(1)
		}
		sel, err := config.SelectSchedules(cfg.Schedules, []string{*scheduleName})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		accs, err := config.ResolveScheduleAccounts(sel[0], cfg.Accounts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		email, password = accs[0].Email, accs[0].Password
		if *facilityID == "" {
			facilityIDs = config.ScheduleCourts(sel[0])
		}
		fmt.Printf("Schedule: %s (%s, account %s)\n", sel[0].Name, sel[0].TargetDay, accs[0].Name)
	}

	targetDate := *date
	if targetDate == "" {
		targetDate = time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	}

	fmt.Printf("Courts:   %v\n", facilityIDs)
	fmt.Printf("Date:     %s\n", targetDate)
	fmt.Println()

	// Step 1: Login
	fmt.Println("[1/4] Logging in...")
	client := api.NewClient(cfg.BaseURL)
	if err := client.Login(email, password); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Login successful!")
	fmt.Println()

	// Step 2: Resolve court names to IDs
	fmt.Println("[2/4] Resolving court names...")
	var resolvedFacilityIDs []string
	for _, fid := range facilityIDs {
		resolvedID, err := client.ResolveCourtNameToID(fid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR: %v\n", err)
			os.Exit(1)
		}
		if resolvedID != fid {
			fmt.Printf("  %s -> %s\n", fid, resolvedID)
		}
		resolvedFacilityIDs = append(resolvedFacilityIDs, resolvedID)
	}
	fmt.Println("  All courts resolved!")
	fmt.Println()

	// Step 3: Get timeslots
	fmt.Println("[3/4] Fetching timeslots...")
	for _, fid := range resolvedFacilityIDs {
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
	scheduleName := bookFlags.String("schedule", "", "Schedule name from schedules.yaml (uses its courts + account)")
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

	email, password, defaultCourts := primaryCredentials(cfg)
	facilityIDs := defaultCourts
	if *facilityID != "" {
		facilityIDs = []string{*facilityID}
	}
	bookUnitID, bookName, bookContact := cfg.UnitID, cfg.BookingName, cfg.Contact
	if len(cfg.Accounts) > 0 && (cfg.Email == "" || *scheduleName != "") {
		bookUnitID, bookName, bookContact = cfg.Accounts[0].UnitID, cfg.Accounts[0].BookingName, cfg.Accounts[0].Contact
	}
	if *scheduleName != "" {
		if len(cfg.Schedules) == 0 {
			fmt.Fprintf(os.Stderr, "ERROR: --schedule %q given but no schedules file loaded (%s)\n", *scheduleName, scheduleFileLabel(cfg))
			os.Exit(1)
		}
		sel, err := config.SelectSchedules(cfg.Schedules, []string{*scheduleName})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		accs, err := config.ResolveScheduleAccounts(sel[0], cfg.Accounts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		email, password = accs[0].Email, accs[0].Password
		bookUnitID, bookName, bookContact = accs[0].UnitID, accs[0].BookingName, accs[0].Contact
		if *facilityID == "" {
			facilityIDs = config.ScheduleCourts(sel[0])
		}
		fmt.Printf("Schedule: %s (%s, account %s)\n", sel[0].Name, sel[0].TargetDay, accs[0].Name)
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
	fmt.Println("[1/5] Logging in...")
	client := api.NewClient(cfg.BaseURL)
	if err := client.Login(email, password); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Login successful!")
	fmt.Println()

	// Step 2: Resolve court names to IDs
	fmt.Println("[2/5] Resolving court names...")
	var resolvedFacilityIDs []string
	for _, fid := range facilityIDs {
		resolvedID, err := client.ResolveCourtNameToID(fid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERROR: %v\n", err)
			os.Exit(1)
		}
		if resolvedID != fid {
			fmt.Printf("  %s -> %s\n", fid, resolvedID)
		}
		resolvedFacilityIDs = append(resolvedFacilityIDs, resolvedID)
	}
	fmt.Println("  All courts resolved!")
	fmt.Println()

	// Step 3: Find an available court for the requested slot
	fmt.Println("[3/5] Checking availability...")
	var availableCourt string
	for _, fid := range resolvedFacilityIDs {
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

	// Step 4: Book the slot
	fmt.Println("[4/5] Booking slot...")
	fmt.Printf("  Booking court %s, %s on %s...\n", availableCourt, *timeSlot, targetDate)
	result, err := client.BookSlot(availableCourt, bookUnitID, bookName, bookContact, targetDate, *timeSlot)
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
	listSchedules := runFlags.Bool("list-schedules", false, "List schedules from schedules.yaml and exit")
	var scheduleNames scheduleFlag
	runFlags.Var(&scheduleNames, "schedule", "Run only this schedule (repeatable, default: all)")
	runFlags.Parse(os.Args[2:])

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	if cfg.ScheduleFile != "" {
		fmt.Printf("Schedules:  %s\n", cfg.ScheduleFile)
	}

	if *listSchedules {
		printSchedules(cfg)
		return
	}

	// runUnit is one schedule × one account with the schedule's plan.
	// Schedule plan wins; the account's own BookingPlan is ignored in schedule mode.
	type runUnit struct {
		schedule string
		day      time.Weekday
		account  config.Account
		plan     []config.BookingEntry
	}
	var units []runUnit
	var skipped []string
	useSchedules := len(cfg.Schedules) > 0

	// Calculate target date: today + 7 days (the date that opens at next midnight)
	today := time.Now()
	targetDate := today.AddDate(0, 0, 7).Format("2006-01-02")
	targetDateParsed, _ := time.Parse("2006-01-02", targetDate)

	if useSchedules {
		selected, err := config.SelectSchedules(cfg.Schedules, []string(scheduleNames))
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		for _, s := range selected {
			day, err := parseDayOfWeek(s.TargetDay)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR schedule %q: %v\n", s.Name, err)
				os.Exit(1)
			}
			if targetDateParsed.Weekday() != day && !*now {
				skipped = append(skipped, fmt.Sprintf("%s (target %s is %s, schedule wants %s)", s.Name, targetDate, targetDateParsed.Weekday(), day))
				continue
			}
			accs, err := config.ResolveScheduleAccounts(s, cfg.Accounts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
				os.Exit(1)
			}
			for _, acc := range accs {
				units = append(units, runUnit{schedule: s.Name, day: day, account: acc, plan: s.BookingPlan})
			}
		}
		if len(units) == 0 {
			fmt.Printf("Nothing to book tonight — all selected schedules skipped:\n")
			for _, sk := range skipped {
				fmt.Printf("  - %s\n", sk)
			}
			fmt.Println("Run with --now to override and book anyway.")
			return
		}
	} else {
		if len(scheduleNames) > 0 {
			fmt.Fprintf(os.Stderr, "ERROR: --schedule given but no schedules file loaded (%s)\n", scheduleFileLabel(cfg))
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

		// Check if the target date falls on the desired day of week
		if targetDateParsed.Weekday() != targetDayOfWeek {
			fmt.Printf("Nothing to book tonight — %s is a %s, not %s.\n",
				targetDate, targetDateParsed.Weekday(), targetDayOfWeek)
			fmt.Println("Run with --now to override and book anyway.")
			if !*now {
				notifyMsg := fmt.Sprintf("Court bot skipped: target date %s is %s (expected %s)", targetDate, targetDateParsed.Weekday(), targetDayOfWeek)
				if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
					if err := sendTelegramMessage(cfg.TelegramBotToken, cfg.TelegramChatID, notifyMsg); err != nil {
						fmt.Fprintf(os.Stderr, "WARN: failed to send Telegram notification: %v\n", err)
					}
				}
				return
			}
			fmt.Println("--now flag set, proceeding anyway...")
			fmt.Println()
		}
		for _, acc := range cfg.Accounts {
			units = append(units, runUnit{schedule: "", day: targetDayOfWeek, account: acc, plan: acc.BookingPlan})
		}
	}

	// Check that at least one unit has a booking plan
	totalSlots := 0
	for _, u := range units {
		totalSlots += len(u.plan)
	}
	if totalSlots == 0 {
		fmt.Fprintf(os.Stderr, "ERROR: No booking plans configured. Set GPROP_BOOKING_PLAN or GPROP_ACCOUNT_N_BOOKING_PLAN\n")
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

	if useSchedules {
		fmt.Printf("Target date:  %s (%s)\n", targetDate, targetDateParsed.Weekday())
		if len(skipped) > 0 {
			fmt.Printf("Skipped:      %d schedule(s)\n", len(skipped))
			for _, sk := range skipped {
				fmt.Printf("  - %s\n", sk)
			}
		}
		for _, u := range units {
			fmt.Printf("\n  [%s / %s]\n", u.schedule, u.account.Name)
			for _, entry := range u.plan {
				fmt.Printf("    %s → courts %v\n", entry.Slot, entry.Courts)
			}
		}
	} else {
		fmt.Printf("Target day:   %s\n", cfg.TargetDay)
		fmt.Printf("Target date:  %s (%s)\n", targetDate, targetDateParsed.Weekday())
		fmt.Printf("Accounts:     %d\n", len(cfg.Accounts))
		for i, acc := range cfg.Accounts {
			fmt.Printf("\n  [Account %d: %s]\n", i+1, acc.Name)
			for _, entry := range acc.BookingPlan {
				fmt.Printf("    %s → courts %v\n", entry.Slot, entry.Courts)
			}
		}
	}
	if *dryRun {
		fmt.Println("\nMode:         DRY RUN")
	}
	fmt.Println()

	// Unique accounts across units (a schedule may share accounts).
	var uniqAccounts []config.Account
	seenAcc := map[string]bool{}
	for _, u := range units {
		if !seenAcc[u.account.Email] {
			seenAcc[u.account.Email] = true
			uniqAccounts = append(uniqAccounts, u.account)
		}
	}

	// Step 1: Login and resolve courts
	fmt.Println("[1/3] Logging in and resolving court names...")
	clients := map[string]*api.Client{}
	for _, acc := range uniqAccounts {
		fmt.Printf("  %s: logging in... ", acc.Name)
		client := api.NewClient(cfg.BaseURL)
		if err := client.Login(acc.Email, acc.Password); err != nil {
			notify(fmt.Sprintf("Court bot error: login failed for %s - %v", acc.Name, err))
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("OK")

		clients[acc.Email] = client
	}
	fmt.Println()

	// Step 2: Wait for midnight (unless --now)
	pollAttempts := 0
	var fireTime time.Time
	var midnight time.Time
	if !*now {
		midnight = time.Date(today.Year(), today.Month(), today.Day()+1, 0, 0, 0, 0, today.Location())
		waitDuration := time.Until(midnight)

		// Re-login at 23:59:30 if more than 60s away
		if waitDuration > 60*time.Second {
			preLoginWait := waitDuration - 30*time.Second
			fmt.Printf("  Sleeping %s, then re-authenticating...\n", preLoginWait.Round(time.Second))
			time.Sleep(preLoginWait)
			fmt.Println("  Re-authenticating all accounts...")
			for _, acc := range uniqAccounts {
				// Login now retries internally (Task 2), just log attempt
				fmt.Printf("  %s: re-login... ", acc.Name)
				if err := clients[acc.Email].Login(acc.Email, acc.Password); err != nil {
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
		consecutiveErrors := 0
		ticker := time.NewTicker(200 * time.Millisecond)

		// Poll primary facility first (fast check) — resolve once before loop
		primaryCourt := "" // first court of first unit's first plan entry
		var pollClient *api.Client
		targetSlot := ""
		if len(units) > 0 && len(units[0].plan) > 0 &&
			len(units[0].plan[0].Courts) > 0 {
			primaryCourt = units[0].plan[0].Courts[0]
			targetSlot = units[0].plan[0].Slot
			pollClient = clients[units[0].account.Email]
		}
		if primaryCourt == "" {
			fmt.Fprintln(os.Stderr, "ERROR: no courts configured for poll, proceeding to midnight wait")
		}
		resolvedID := primaryCourt
		if !api.IsNumeric(resolvedID) && resolvedID != "" && pollClient != nil {
			if rid, err := pollClient.ResolveCourtNameToID(resolvedID); err == nil {
				resolvedID = rid
			} else {
				fmt.Fprintf(os.Stderr, "WARN: failed to resolve poll court %q: %v — polls will likely fail, continuing to midnight\n", resolvedID, err)
			}
		}

		pollTimeout := midnight.Add(30 * time.Second)
		for range ticker.C {
			pollAttempts++
			if time.Now().After(pollTimeout) {
				fmt.Println("  Poll timeout 00:00:30, proceeding to book anyway")
				break
			}

			if pollClient == nil {
				fmt.Println("  No poll client, proceeding to book anyway")
				break
			}
			slots, err := pollClient.GetTimeslots(resolvedID, targetDate)
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

			// Check if target slot available (first plan slot of first unit)
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
		}
		ticker.Stop()

		// Final wait until exact midnight if we broke early before midnight.
		// Never book before midnight — clamp early flip to midnight.
		if fireTime.IsZero() {
			remaining := time.Until(midnight)
			if remaining > 0 {
				fmt.Printf("  Final wait: %s\n", remaining.Round(time.Millisecond))
				time.Sleep(remaining)
			}
			fireTime = time.Now()
			fmt.Printf("  MIDNIGHT! %s (polls=%d)\n", fireTime.Format("15:04:05.000"), pollAttempts)
		} else if fireTime.Before(midnight) {
			remaining := time.Until(midnight)
			fmt.Printf("  Early flip at %s, waiting %s until midnight...\n", fireTime.Format("15:04:05.000"), remaining.Round(time.Millisecond))
			if remaining > 0 {
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
	fmt.Println()

	// Step 3: Book slots for each unit (schedule × account)
	fmt.Println("[3/3] Booking target slots...")
	totalSuccess := 0
	for _, u := range units {
		acc := u.account
		if len(u.plan) == 0 {
			continue
		}

		if useSchedules {
			fmt.Printf("\n=== %s / %s ===\n", u.schedule, acc.Name)
		} else {
			fmt.Printf("\n=== %s ===\n", acc.Name)
		}
		client := clients[acc.Email]
		successCount := 0

		for _, entry := range u.plan {
			// Resolve court names to IDs for this entry
			var resolvedCourts []string
			for _, court := range entry.Courts {
				resolvedID, err := client.ResolveCourtNameToID(court)
				if err != nil {
					fmt.Printf("    ERROR resolving court %s: %v\n", court, err)
					continue
				}
				if resolvedID != court {
					fmt.Printf("    Resolved: %s -> %s\n", court, resolvedID)
				}
				resolvedCourts = append(resolvedCourts, resolvedID)
			}

			if len(resolvedCourts) == 0 {
				fmt.Printf("    ERROR: No valid courts for slot %s\n", entry.Slot)
				continue
			}

			fmt.Printf("\n  Slot: %s (courts: %v)\n", entry.Slot, resolvedCourts)
			booked := false

			for _, fid := range resolvedCourts {
				if *dryRun {
					fmt.Printf("    [DRY RUN] Would try court %s for %s on %s\n", fid, entry.Slot, targetDate)
					continue
				}

				// Try booking with retries
				var lastErr error
				for attempt := 1; attempt <= 3; attempt++ {
					if attempt > 1 {
						backoff := time.Duration(1<<(attempt-2)) * time.Second
						fmt.Printf("    Retry %d after %s...\n", attempt, backoff)
						time.Sleep(backoff)
					}

					result, err := client.BookSlot(fid, acc.UnitID, acc.BookingName, acc.Contact, targetDate, entry.Slot)
					if err != nil {
						lastErr = err
						fmt.Printf("    Court %s attempt %d: ERROR %v\n", fid, attempt, err)
						continue
					}

					if result.Status {
						fmt.Printf("    Court %s: SUCCESS — %s (ID: %d)\n", fid, result.Msg, result.InsertID)
						booked = true
						successCount++
						break
					} else {
						fmt.Printf("    Court %s: REJECTED — %s\n", fid, result.Msg)
						lastErr = fmt.Errorf("%s", result.Msg)
						break // Don't retry on server rejection, try next court
					}
				}

				if booked {
					break // Move to next slot
				}
				if lastErr != nil {
					fmt.Printf("    Court %s: failed — %v\n", fid, lastErr)
				}
			}

			if !booked && !*dryRun {
				fmt.Printf("    FAILED to book %s on any court\n", entry.Slot)
			}
		}

		fmt.Printf("\n  %s: %d/%d slots booked\n", acc.Name, successCount, len(u.plan))
		totalSuccess += successCount
	}

	fmt.Println()
	fireStr := fireTime.Format("15:04:05.000")
	fireDelayStr := "0s"
	if fireTime.IsZero() {
		fireStr = time.Now().Format("15:04:05.000")
		if !midnight.IsZero() {
			fireDelayStr = time.Since(midnight).Round(time.Millisecond).String()
		}
	} else {
		if !midnight.IsZero() {
			fireDelayStr = fireTime.Sub(midnight).Round(time.Millisecond).String()
		} else {
			fireDelayStr = "0s"
		}
	}
	if *dryRun {
		fmt.Println("=== DRY RUN complete ===")
		notify(fmt.Sprintf("Court bot dry run complete for %s (%d accounts, %d total slots) (polls=%d fire=%s delay=%s)", targetDate, len(uniqAccounts), totalSlots, pollAttempts, fireStr, fireDelayStr))
	} else {
		fmt.Printf("=== Done: %d/%d total slots booked ===\n", totalSuccess, totalSlots)
		notify(fmt.Sprintf("Court bot done for %s: %d/%d slots booked (polls=%d fire=%s delay=%s)", targetDate, totalSuccess, totalSlots, pollAttempts, fireStr, fireDelayStr))
	}
}

func printSchedules(cfg *config.Config) {
	if len(cfg.Schedules) == 0 {
		fmt.Printf("No schedules file loaded (%s).\n", scheduleFileLabel(cfg))
		fmt.Printf("Target day: %s\n", cfg.TargetDay)
		for _, acc := range cfg.Accounts {
			fmt.Printf("\n  [%s]\n", acc.Name)
			for _, entry := range acc.BookingPlan {
				fmt.Printf("    %s → courts %v\n", entry.Slot, entry.Courts)
			}
		}
		return
	}
	fmt.Printf("Schedules (%d) from %s:\n", len(cfg.Schedules), cfg.ScheduleFile)
	for _, s := range cfg.Schedules {
		accs, err := config.ResolveScheduleAccounts(s, cfg.Accounts)
		names := ""
		if err != nil {
			names = "ERROR: " + err.Error()
		} else {
			for i, a := range accs {
				if i > 0 {
					names += ", "
				}
				names += a.Name
			}
		}
		fmt.Printf("\n  %s: day=%s accounts=[%s]\n", s.Name, s.TargetDay, names)
		for _, entry := range s.BookingPlan {
			fmt.Printf("    %s → courts %v\n", entry.Slot, entry.Courts)
		}
	}
}

func sendTelegramMessage(botToken, chatID, message string) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.PostForm(endpoint, url.Values{
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
	httpClient := &http.Client{Timeout: 45 * time.Second}
	resp, err := httpClient.Get(endpoint)
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

	// Build account/plan summary
	var planStr string
	totalSlots := 0
	for _, acc := range cfg.Accounts {
		if len(acc.BookingPlan) == 0 {
			continue
		}
		planStr += fmt.Sprintf("\n\n👤 %s", acc.Name)
		for _, entry := range acc.BookingPlan {
			planStr += fmt.Sprintf("\n  • %s → %v", entry.Slot, entry.Courts)
			totalSlots++
		}
	}

	status := fmt.Sprintf(`🥒🎾 Court Bot Status

📅 Target day: %s
📆 Next booking date: %s (%s)
⏰ Next cron run: %s
⏱ Time until run: %s
👥 Accounts: %d (%d slots)

📋 Booking plan:%s

✅ Bot is running`,
		cfg.TargetDay,
		targetDate,
		targetDateParsed.Weekday(),
		nextRun.Format("Mon Jan 2, 15:04"),
		time.Until(nextRun).Round(time.Minute),
		len(cfg.Accounts),
		totalSlots,
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
	msg := "🥒🎾 Upcoming Bookings\n"
	totalBookings := 0

	for _, acc := range cfg.Accounts {
		client := api.NewClient(cfg.BaseURL)
		if err := client.Login(acc.Email, acc.Password); err != nil {
			msg += fmt.Sprintf("\n👤 %s\n❌ Login failed: %v\n", acc.Name, err)
			continue
		}

		bookings, err := client.GetBookings()
		if err != nil {
			msg += fmt.Sprintf("\n👤 %s\n❌ Failed to fetch: %v\n", acc.Name, err)
			continue
		}

		if len(bookings) == 0 {
			continue
		}

		msg += fmt.Sprintf("\n👤 %s\n", acc.Name)
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

			msg += fmt.Sprintf("%s %s | %s-%s | %s\n",
				statusEmoji, dateStr, timeStart, timeEnd, b.Facility)
			totalBookings++
		}
	}

	if totalBookings == 0 {
		msg = "📋 No upcoming bookings found."
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

func cmdFacilities() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[1/2] Logging in...")
	client := api.NewClient(cfg.BaseURL)
	email, password, _ := primaryCredentials(cfg)
	if err := client.Login(email, password); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Login successful!")
	fmt.Println()

	fmt.Println("[2/2] Fetching facilities...")
	facilities, err := client.GetFacilities()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Available Courts:")
	fmt.Println("  ID    Name")
	fmt.Println("  ----  " + strings.Repeat("-", 40))
	for _, f := range facilities {
		fmt.Printf("  %-4s  %s\n", f.ID, f.Name)
	}
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
		return time.Sunday, fmt.Errorf("invalid day: %s", s)
	}
	return day, nil
}

func cmdHealthCheck() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[1/2] Testing login...")
	client := api.NewClient(cfg.BaseURL)
	hcEmail, hcPassword, _ := primaryCredentials(cfg)
	if err := client.Login(hcEmail, hcPassword); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Login failed: %v\n", err)
		fmt.Println("[2/2] Sending alert to Telegram...")

		// Send Telegram notification on failure
		if cfg.TelegramBotToken != "" && cfg.TelegramChatID != "" {
			message := fmt.Sprintf("🚨 Court Bot Health Check Failed\n\nLogin failed for court booking bot.\n\nError: %v\n\nTime: %s", err, time.Now().Format("2006-01-02 15:04:05"))
			if err := sendTelegramMessage(cfg.TelegramBotToken, cfg.TelegramChatID, message); err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: Failed to send Telegram notification: %v\n", err)
			} else {
				fmt.Println("  Alert sent successfully")
			}
		} else {
			fmt.Println("  WARNING: Telegram credentials not set, skipping notification")
		}

		os.Exit(1)
	}

	fmt.Println("  Login successful!")
	fmt.Println("[2/2] Health check passed")
	fmt.Println("✓ Login is working correctly")
}

func cmdServe() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	if cfg.UIPassword == "" {
		fmt.Fprintln(os.Stderr, "ERROR: UI_PASSWORD must be set to run the web UI")
		os.Exit(1)
	}
	if len(cfg.UIPassword) < 16 {
		fmt.Fprintln(os.Stderr, "ERROR: UI_PASSWORD must be at least 16 characters")
		os.Exit(1)
	}
	if len(cfg.Accounts) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR: no accounts configured")
		os.Exit(1)
	}
	srv := web.NewServer(web.ServerOptions{
		Password: cfg.UIPassword,
		Backend:  web.NewLiveBackend(cfg),
	})
	addr := cfg.UIBind + ":" + cfg.UIPort
	fmt.Printf("Court Bot UI on http://%s\n", addr)
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	if err := httpSrv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
