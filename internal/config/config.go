package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// BookingEntry represents a single slot with its court priority order.
type BookingEntry struct {
	Slot   string   // e.g. "07:00-08:00"
	Courts []string // ordered by preference, e.g. ["7937", "7935"]
}

// Account represents a single booking account with its own credentials and plan.
type Account struct {
	Name        string // display name for this account
	Email       string
	Password    string
	UnitID      string
	BookingName string
	Contact     string
	BookingPlan []BookingEntry
}

// Config holds the application configuration.
type Config struct {
	// Legacy single-account fields (still supported for backwards compatibility)
	Email       string
	Password    string
	FacilityIDs []string
	BaseURL     string
	UnitID      string
	BookingName string
	Contact     string
	TargetDay   string         // e.g. "friday"
	BookingPlan []BookingEntry // parsed from GPROP_BOOKING_PLAN

	// Multi-account support
	Accounts []Account

	TelegramBotToken string
	TelegramChatID   string
}

// parseBookingPlan parses a booking plan string like "07:00-08:00>7937,7936;08:00-09:00>7937"
func parseBookingPlan(raw string) ([]BookingEntry, error) {
	var plan []BookingEntry
	if raw == "" {
		return plan, nil
	}
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ">", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid booking plan entry: %q (expected slot>court1,court2)", entry)
		}
		slot := strings.TrimSpace(parts[0])
		var courts []string
		for _, c := range strings.Split(parts[1], ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				courts = append(courts, c)
			}
		}
		if slot != "" && len(courts) > 0 {
			plan = append(plan, BookingEntry{Slot: slot, Courts: courts})
		}
	}
	return plan, nil
}

// Load reads configuration from .env file and environment variables.
func Load() (*Config, error) {
	// Load .env file if it exists (does not override existing env vars)
	_ = godotenv.Load()

	cfg := &Config{
		Email:            os.Getenv("GPROP_EMAIL"),
		Password:         os.Getenv("GPROP_PASSWORD"),
		BaseURL:          os.Getenv("GPROP_BASE_URL"),
		UnitID:           os.Getenv("GPROP_UNIT_ID"),
		BookingName:      os.Getenv("GPROP_BOOKING_NAME"),
		Contact:          os.Getenv("GPROP_CONTACT"),
		TelegramBotToken: strings.TrimSpace(os.Getenv("GPROP_TELEGRAM_BOT_TOKEN")),
		TelegramChatID:   strings.TrimSpace(os.Getenv("GPROP_TELEGRAM_CHAT_ID")),
	}

	// Parse comma-separated facility IDs
	rawIDs := os.Getenv("GPROP_FACILITY_IDS")
	if rawIDs != "" {
		for _, id := range strings.Split(rawIDs, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				cfg.FacilityIDs = append(cfg.FacilityIDs, id)
			}
		}
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://www.gpropsystems.com"
	}

	// Parse target day
	cfg.TargetDay = strings.ToLower(strings.TrimSpace(os.Getenv("GPROP_TARGET_DAY")))

	// Check for multi-account config (GPROP_ACCOUNT_1_EMAIL, GPROP_ACCOUNT_2_EMAIL, etc.)
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("GPROP_ACCOUNT_%d_", i)
		email := os.Getenv(prefix + "EMAIL")
		if email == "" {
			break // No more accounts
		}

		password := os.Getenv(prefix + "PASSWORD")
		if password == "" {
			return nil, fmt.Errorf("%sPASSWORD must be set", prefix)
		}

		// Account-specific fields with fallback to global config
		unitID := os.Getenv(prefix + "UNIT_ID")
		if unitID == "" {
			unitID = cfg.UnitID
		}
		bookingName := os.Getenv(prefix + "BOOKING_NAME")
		if bookingName == "" {
			bookingName = cfg.BookingName
		}
		contact := os.Getenv(prefix + "CONTACT")
		if contact == "" {
			contact = cfg.Contact
		}

		// Parse account-specific booking plan
		rawPlan := os.Getenv(prefix + "BOOKING_PLAN")
		plan, err := parseBookingPlan(rawPlan)
		if err != nil {
			return nil, fmt.Errorf("account %d: %w", i, err)
		}

		// Account name defaults to "Account N"
		name := os.Getenv(prefix + "NAME")
		if name == "" {
			name = fmt.Sprintf("Account %d", i)
		}

		cfg.Accounts = append(cfg.Accounts, Account{
			Name:        name,
			Email:       email,
			Password:    password,
			UnitID:      unitID,
			BookingName: bookingName,
			Contact:     contact,
			BookingPlan: plan,
		})
	}

	// If no multi-account config, use legacy single-account config
	if len(cfg.Accounts) == 0 {
		if cfg.Email == "" || cfg.Password == "" {
			return nil, fmt.Errorf("GPROP_EMAIL and GPROP_PASSWORD must be set (or use GPROP_ACCOUNT_1_EMAIL, etc.)")
		}
		if len(cfg.FacilityIDs) == 0 {
			return nil, fmt.Errorf("GPROP_FACILITY_IDS must be set")
		}
		if cfg.UnitID == "" || cfg.BookingName == "" || cfg.Contact == "" {
			return nil, fmt.Errorf("GPROP_UNIT_ID, GPROP_BOOKING_NAME, and GPROP_CONTACT must be set")
		}

		// Parse legacy booking plan
		plan, err := parseBookingPlan(os.Getenv("GPROP_BOOKING_PLAN"))
		if err != nil {
			return nil, err
		}
		cfg.BookingPlan = plan

		// Create a single account from legacy config
		cfg.Accounts = append(cfg.Accounts, Account{
			Name:        "Primary",
			Email:       cfg.Email,
			Password:    cfg.Password,
			UnitID:      cfg.UnitID,
			BookingName: cfg.BookingName,
			Contact:     cfg.Contact,
			BookingPlan: plan,
		})
	}

	return cfg, nil
}
