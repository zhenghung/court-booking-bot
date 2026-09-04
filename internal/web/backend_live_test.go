package web

import (
	"testing"
	"time"

	"github.com/zhenghung/court-booking-bot/internal/api"
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

func TestResolveCourtExactOnly(t *testing.T) {
	fac := []api.Facility{{ID: "7935", Name: "Pickleball Court P1"}}
	if id, err := resolveCourt(fac, "P1", false); err != nil || id != "7935" {
		t.Fatalf("partial probe resolve = %q, %v", id, err)
	}
	if _, err := resolveCourt(fac, "P1", true); err == nil {
		t.Fatal("exact-only book resolve should reject partial match")
	}
	if id, err := resolveCourt(fac, "pickleball court p1", true); err != nil || id != "7935" {
		t.Fatalf("exact book resolve = %q, %v", id, err)
	}
	if _, err := NewLiveBackend(&config.Config{Accounts: []config.Account{{Name: "T"}}}).Book(BookRequest{Date: "2026-09-12", Time: "07:00-08:00", DryRun: true}); err != ErrNoCourts {
		t.Fatalf("empty courts book err = %v, want ErrNoCourts", err)
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
