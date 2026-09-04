package web

import (
	"testing"
	"time"

	"github.com/zhenghung/court-booking-bot/internal/config"
)

func TestLiveBackendEmptyAccountsNoPanic(t *testing.T) {
	b := NewLiveBackend(&config.Config{})
	if _, err := b.Facilities(); err == nil {
		t.Fatal("Facilities with no accounts should error")
	}
	if _, err := b.Probe("2026-09-12", nil); err == nil {
		t.Fatal("Probe with no accounts should error")
	}
	if _, err := b.Book(BookRequest{Date: "2026-09-12", Time: "07:00-08:00", DryRun: true}); err == nil {
		t.Fatal("Book with no accounts should error")
	}
	// Status must still work (renders empty accounts).
	if s := b.Status(); len(s.Accounts) != 0 {
		t.Fatalf("Status accounts = %d, want 0", len(s.Accounts))
	}
}

func TestNextRunFriday(t *testing.T) {
	// Friday 2026-09-04 12:00 KL -> next run is same-day midnight? No:
	// midnight today passed, so next Friday midnight (7 days out).
	kl := time.FixedZone("MYT", 8*3600)
	fridayNoon := time.Date(2026, 9, 4, 12, 0, 0, 0, kl)
	if got := nextRun(fridayNoon, "friday"); got != "Fri Sep 11, 00:00" {
		t.Fatalf("nextRun fri noon = %q, want Fri Sep 11, 00:00", got)
	}
	// Thursday -> next-day Friday midnight.
	thursday := time.Date(2026, 9, 3, 12, 0, 0, 0, kl)
	if got := nextRun(thursday, "friday"); got != "Fri Sep 4, 00:00" {
		t.Fatalf("nextRun thu = %q, want Fri Sep 4, 00:00", got)
	}
	// Bad day -> empty.
	if got := nextRun(thursday, "funday"); got != "" {
		t.Fatalf("nextRun bad day = %q, want empty", got)
	}
}
