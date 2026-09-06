package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Schedule is one independent sniping schedule loaded from schedules.yaml.
type Schedule struct {
	Name         string
	TargetDay    string // lowercase, e.g. "friday"
	BookingPlan  []BookingEntry
	AccountNames []string // ["all"] or account names
}

var scheduleSlugRe = regexp.MustCompile(`^[a-z0-9-]+$`)
var scheduleSlotRe = regexp.MustCompile(`^\d{2}:\d{2}-\d{2}:\d{2}$`)

var validDays = map[string]bool{
	"sunday": true, "sun": true,
	"monday": true, "mon": true,
	"tuesday": true, "tue": true,
	"wednesday": true, "wed": true,
	"thursday": true, "thu": true,
	"friday": true, "fri": true,
	"saturday": true, "sat": true,
}

type schedulesFile struct {
	Schedules []scheduleYAML `yaml:"schedules"`
}

type scheduleYAML struct {
	Name        string   `yaml:"name"`
	TargetDay   string   `yaml:"target_day"`
	BookingPlan string   `yaml:"booking_plan"`
	Accounts    []string `yaml:"accounts"`
}

// LoadSchedulesFile parses and validates a schedules.yaml file.
// Missing file returns empty schedules with no error (legacy env path stays noop).
func LoadSchedulesFile(path string, accounts []Account) ([]Schedule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read schedules file: %w", err)
	}
	var raw schedulesFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse schedules file %s: %w", path, err)
	}
	seen := map[string]bool{}
	var out []Schedule
	for i, s := range raw.Schedules {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			return nil, fmt.Errorf("schedule %d: name must be set", i+1)
		}
		if !scheduleSlugRe.MatchString(name) {
			return nil, fmt.Errorf("schedule %q: name must match [a-z0-9-]+ (no spaces)", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("schedule %q: duplicate name", name)
		}
		seen[name] = true

		day := strings.ToLower(strings.TrimSpace(s.TargetDay))
		if !validDays[day] {
			return nil, fmt.Errorf("schedule %q: invalid target_day %q", name, s.TargetDay)
		}

		plan, err := parseBookingPlan(s.BookingPlan)
		if err != nil {
			return nil, fmt.Errorf("schedule %q: %w", name, err)
		}
		if len(plan) == 0 {
			return nil, fmt.Errorf("schedule %q: booking_plan must have at least one entry", name)
		}

		if len(s.Accounts) == 0 {
			return nil, fmt.Errorf("schedule %q: accounts must be set ([all] or account names)", name)
		}
		var names []string
		for _, a := range s.Accounts {
			a = strings.TrimSpace(a)
			if a != "" {
				names = append(names, a)
			}
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("schedule %q: accounts must be set ([all] or account names)", name)
		}
		// Validate account refs against known accounts.
		hasAll := false
		for _, n := range names {
			if n == "all" {
				hasAll = true
				break
			}
		}
		if hasAll && len(names) > 1 {
			return nil, fmt.Errorf("schedule %q: accounts cannot mix \"all\" with names", name)
		}
		if !hasAll {
			known := map[string]bool{}
			for _, acc := range accounts {
				known[acc.Name] = true
			}
			for _, n := range names {
				if !known[n] {
					return nil, fmt.Errorf("schedule %q: unknown account %q", name, n)
				}
			}
		}

		out = append(out, Schedule{
			Name:         name,
			TargetDay:    day,
			BookingPlan:  plan,
			AccountNames: names,
		})
	}
	return out, nil
}

// validateSlotRange rejects impossible ranges like 99:99-99:99 or 09:00-08:00.
// time.Parse enforces HH 00-23 and MM 00-59; we additionally require start < end.
func validateSlotRange(slot string) error {
	parts := strings.SplitN(slot, "-", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected HH:MM-HH:MM")
	}
	start, err := time.Parse("15:04", parts[0])
	if err != nil {
		return fmt.Errorf("bad start time: %w", err)
	}
	end, err := time.Parse("15:04", parts[1])
	if err != nil {
		return fmt.Errorf("bad end time: %w", err)
	}
	if !start.Before(end) {
		return fmt.Errorf("start must be before end")
	}
	return nil
}

// SelectSchedules filters schedules by name. Empty names returns all.
// Duplicate names are deduped so a schedule never books twice.
func SelectSchedules(all []Schedule, names []string) ([]Schedule, error) {
	if len(names) == 0 {
		return all, nil
	}
	byName := map[string]Schedule{}
	for _, s := range all {
		byName[s.Name] = s
	}
	var out []Schedule
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		s, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("unknown schedule %q", n)
		}
		out = append(out, s)
	}
	return out, nil
}

// ResolveScheduleAccounts maps a schedule's account refs to actual accounts.
func ResolveScheduleAccounts(s Schedule, accounts []Account) ([]Account, error) {
	for _, n := range s.AccountNames {
		if n == "all" {
			return accounts, nil
		}
	}
	var out []Account
	for _, n := range s.AccountNames {
		found := false
		for _, acc := range accounts {
			if acc.Name == n {
				out = append(out, acc)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("schedule %q: unknown account %q", s.Name, n)
		}
	}
	return out, nil
}

// ScheduleCourts returns the union of courts across a schedule's plan, order-preserved.
func ScheduleCourts(s Schedule) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range s.BookingPlan {
		for _, c := range e.Courts {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// resolveScheduleFilePath returns the schedules file to load, or "" if none exists.
// Order: GPROP_SCHEDULES_FILE -> ./schedules.yaml -> ~/.schedules.yaml.
func resolveScheduleFilePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if _, err := os.Stat("schedules.yaml"); err == nil {
		return "schedules.yaml"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		p := filepath.Join(home, ".schedules.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
