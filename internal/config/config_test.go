package config

import (
	"os"
	"path/filepath"
	"testing"
)

func testAccounts() []Account {
	return []Account{
		{Name: "Account 1", Email: "a1@example.com"},
		{Name: "Account 2", Email: "a2@example.com"},
	}
}

func writeSchedulesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schedules.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSchedulesFileValid(t *testing.T) {
	path := writeSchedulesFile(t, `schedules:
  - name: fri-pickle
    target_day: friday
    booking_plan: "07:00-09:00>P3,P1,P2"
    accounts: [all]
  - name: mon-badminton
    target_day: monday
    booking_plan: "18:00-20:00>Court A,Court B"
    accounts: ["Account 1"]
`)
	schedules, err := LoadSchedulesFile(path, testAccounts())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(schedules) != 2 {
		t.Fatalf("expected 2 schedules, got %d", len(schedules))
	}
	if schedules[0].Name != "fri-pickle" {
		t.Errorf("expected first schedule fri-pickle, got %q", schedules[0].Name)
	}
	if schedules[0].TargetDay != "friday" {
		t.Errorf("expected target day friday, got %q", schedules[0].TargetDay)
	}
	if len(schedules[0].BookingPlan) != 1 || schedules[0].BookingPlan[0].Slot != "07:00-09:00" {
		t.Errorf("unexpected booking plan: %+v", schedules[0].BookingPlan)
	}
	if len(schedules[0].AccountNames) != 1 || schedules[0].AccountNames[0] != "all" {
		t.Errorf("unexpected account names: %v", schedules[0].AccountNames)
	}
}

func TestLoadSchedulesFileDuplicateName(t *testing.T) {
	path := writeSchedulesFile(t, `schedules:
  - name: fri-pickle
    target_day: friday
    booking_plan: "07:00-09:00>P3"
    accounts: [all]
  - name: fri-pickle
    target_day: monday
    booking_plan: "18:00-20:00>Court A"
    accounts: [all]
`)
	if _, err := LoadSchedulesFile(path, testAccounts()); err == nil {
		t.Fatal("expected duplicate name error, got nil")
	}
}

func TestLoadSchedulesFileBadDay(t *testing.T) {
	path := writeSchedulesFile(t, `schedules:
  - name: fri-pickle
    target_day: funday
    booking_plan: "07:00-09:00>P3"
    accounts: [all]
`)
	if _, err := LoadSchedulesFile(path, testAccounts()); err == nil {
		t.Fatal("expected invalid day error, got nil")
	}
}

func TestLoadSchedulesFileBadPlan(t *testing.T) {
	path := writeSchedulesFile(t, `schedules:
  - name: fri-pickle
    target_day: friday
    booking_plan: "not-a-plan"
    accounts: [all]
`)
	if _, err := LoadSchedulesFile(path, testAccounts()); err == nil {
		t.Fatal("expected invalid plan error, got nil")
	}
}

func TestLoadSchedulesFileUnknownAccount(t *testing.T) {
	path := writeSchedulesFile(t, `schedules:
  - name: fri-pickle
    target_day: friday
    booking_plan: "07:00-09:00>P3"
    accounts: ["Nobody"]
`)
	if _, err := LoadSchedulesFile(path, testAccounts()); err == nil {
		t.Fatal("expected unknown account error, got nil")
	}
}

func TestLoadSchedulesFileAllMixed(t *testing.T) {
	path := writeSchedulesFile(t, `schedules:
  - name: fri-pickle
    target_day: friday
    booking_plan: "07:00-09:00>P3"
    accounts: ["all", "Account 1"]
`)
	if _, err := LoadSchedulesFile(path, testAccounts()); err == nil {
		t.Fatal("expected all-mixed error, got nil")
	}
}

func TestLoadSchedulesFileBadSlotFormat(t *testing.T) {
	path := writeSchedulesFile(t, `schedules:
  - name: fri-pickle
    target_day: friday
    booking_plan: "7-9>P3"
    accounts: [all]
`)
	if _, err := LoadSchedulesFile(path, testAccounts()); err == nil {
		t.Fatal("expected bad slot format error, got nil")
	}
}
func TestLoadExplicitSchedulesFileMissing(t *testing.T) {
	t.Setenv("GPROP_EMAIL", "a@example.com")
	t.Setenv("GPROP_PASSWORD", "pw")
	t.Setenv("GPROP_FACILITY_IDS", "P1")
	t.Setenv("GPROP_UNIT_ID", "1-X")
	t.Setenv("GPROP_BOOKING_NAME", "N")
	t.Setenv("GPROP_CONTACT", "123")
	t.Setenv("GPROP_TARGET_DAY", "friday")
	t.Setenv("GPROP_BOOKING_PLAN", "07:00-09:00>P1")
	t.Setenv("GPROP_SCHEDULES_FILE", filepath.Join(t.TempDir(), "nope.yaml"))
	if _, err := Load(); err == nil {
		t.Fatal("expected explicit missing file error, got nil")
	}
}

func TestLoadSchedulesFileMissing(t *testing.T) {
	schedules, err := LoadSchedulesFile(filepath.Join(t.TempDir(), "nope.yaml"), testAccounts())
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if len(schedules) != 0 {
		t.Fatalf("missing file should yield 0 schedules, got %d", len(schedules))
	}
}

func TestSelectSchedules(t *testing.T) {
	all := []Schedule{
		{Name: "fri-pickle"},
		{Name: "mon-badminton"},
	}
	got, err := SelectSchedules(all, []string{"fri-pickle"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "fri-pickle" {
		t.Fatalf("unexpected selection: %+v", got)
	}
	if _, err := SelectSchedules(all, []string{"nope"}); err == nil {
		t.Fatal("expected unknown schedule error, got nil")
	}
	// Empty filter returns all.
	got, err = SelectSchedules(all, nil)
	if err != nil || len(got) != 2 {
		t.Fatalf("expected all schedules, got %+v err=%v", got, err)
	}
	// Duplicate names dedupe.
	got, err = SelectSchedules(all, []string{"fri-pickle", "fri-pickle"})
	if err != nil || len(got) != 1 {
		t.Fatalf("expected deduped selection, got %+v err=%v", got, err)
	}
}

func TestResolveScheduleAccounts(t *testing.T) {
	accounts := testAccounts()
	s := Schedule{Name: "x", AccountNames: []string{"all"}}
	got, err := ResolveScheduleAccounts(s, accounts)
	if err != nil || len(got) != 2 {
		t.Fatalf("expected all accounts, got %+v err=%v", got, err)
	}
	s = Schedule{Name: "x", AccountNames: []string{"Account 1"}}
	got, err = ResolveScheduleAccounts(s, accounts)
	if err != nil || len(got) != 1 || got[0].Name != "Account 1" {
		t.Fatalf("unexpected accounts: %+v err=%v", got, err)
	}
}

func TestScheduleCourts(t *testing.T) {
	s := Schedule{BookingPlan: []BookingEntry{
		{Slot: "07:00-09:00", Courts: []string{"P3", "P1"}},
		{Slot: "09:00-10:00", Courts: []string{"P1", "P2"}},
	}}
	courts := ScheduleCourts(s)
	if len(courts) != 3 {
		t.Fatalf("expected 3 unique courts, got %v", courts)
	}
}
