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

// Config holds the application configuration.
type Config struct {
	Email       string
	Password    string
	FacilityIDs []string
	BaseURL     string
	UnitID      string
	BookingName string
	Contact     string
	TargetDay   string         // e.g. "friday"
	BookingPlan []BookingEntry // parsed from GPROP_BOOKING_PLAN
	TelegramBotToken string
	TelegramChatID   string
}

// Load reads configuration from .env file and environment variables.
func Load() (*Config, error) {
	// Load .env file if it exists (does not override existing env vars)
	_ = godotenv.Load()

	cfg := &Config{
		Email:       os.Getenv("GPROP_EMAIL"),
		Password:    os.Getenv("GPROP_PASSWORD"),
		BaseURL:     os.Getenv("GPROP_BASE_URL"),
		UnitID:      os.Getenv("GPROP_UNIT_ID"),
		BookingName: os.Getenv("GPROP_BOOKING_NAME"),
		Contact:     os.Getenv("GPROP_CONTACT"),
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

	if cfg.Email == "" || cfg.Password == "" {
		return nil, fmt.Errorf("GPROP_EMAIL and GPROP_PASSWORD must be set in .env or environment")
	}

	if len(cfg.FacilityIDs) == 0 {
		return nil, fmt.Errorf("GPROP_FACILITY_IDS must be set in .env or environment (comma-separated)")
	}

	if cfg.UnitID == "" || cfg.BookingName == "" || cfg.Contact == "" {
		return nil, fmt.Errorf("GPROP_UNIT_ID, GPROP_BOOKING_NAME, and GPROP_CONTACT must be set in .env or environment")
	}

	// Parse target day and booking plan for scheduler
	cfg.TargetDay = strings.ToLower(strings.TrimSpace(os.Getenv("GPROP_TARGET_DAY")))
	rawPlan := os.Getenv("GPROP_BOOKING_PLAN")
	if rawPlan != "" {
		for _, entry := range strings.Split(rawPlan, ";") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			parts := strings.SplitN(entry, ">", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid GPROP_BOOKING_PLAN entry: %q (expected slot>court1,court2)", entry)
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
				cfg.BookingPlan = append(cfg.BookingPlan, BookingEntry{Slot: slot, Courts: courts})
			}
		}
	}

	return cfg, nil
}
