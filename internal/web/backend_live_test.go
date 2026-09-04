package web

import (
	"testing"

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
