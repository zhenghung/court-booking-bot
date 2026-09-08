package web

import (
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/zhenghung/court-booking-bot/internal/api"
	"github.com/zhenghung/court-booking-bot/internal/config"
)

// LiveBackend serves API data from gpropsystems via existing clients.
type LiveBackend struct {
	cfg *config.Config
}

func NewLiveBackend(cfg *config.Config) *LiveBackend {
	return &LiveBackend{cfg: cfg}
}

func (b *LiveBackend) primaryAccount() (config.Account, error) {
	if len(b.cfg.Accounts) == 0 {
		return config.Account{}, fmt.Errorf("no accounts configured")
	}
	return b.cfg.Accounts[0], nil
}

func klNow() time.Time {
	if kl, err := time.LoadLocation("Asia/Kuala_Lumpur"); err == nil {
		return time.Now().In(kl)
	}
	return time.Now().In(time.FixedZone("MYT", 8*3600))
}

func (b *LiveBackend) Status() StatusPayload {
	now := klNow()
	targetDate := now.AddDate(0, 0, 7).Format("2006-01-02")
	var accounts []AccountView
	var plan []string
	for _, acc := range b.cfg.Accounts {
		accounts = append(accounts, AccountView{Name: acc.Name})
		for _, e := range acc.BookingPlan {
			plan = append(plan, e.Slot+" > "+join(e.Courts, ","))
		}
	}
	schedules := b.cfg.GetSchedules()
	var sv []ScheduleView
	for _, s := range schedules {
		sv = append(sv, scheduleToView(s, now))
	}
	return StatusPayload{
		TargetDay:   titleDay(b.cfg.TargetDay),
		TargetDate:  targetDate,
		NextRun:     nextRun(now, b.cfg.TargetDay),
		Accounts:    accounts,
		BookingPlan: plan,
		Schedules:   sv,
	}
}

func scheduleToView(s config.Schedule, now time.Time) ScheduleView {
	var plan []ScheduleEntry
	var courts []string
	seen := map[string]bool{}
	for _, e := range s.BookingPlan {
		plan = append(plan, ScheduleEntry{Slot: e.Slot, Courts: append([]string{}, e.Courts...)})
		for _, c := range e.Courts {
			if !seen[c] {
				seen[c] = true
				courts = append(courts, c)
			}
		}
	}
	return ScheduleView{
		Name:        s.Name,
		TargetDay:   s.TargetDay,
		BookingPlan: plan,
		Accounts:    append([]string{}, s.AccountNames...),
		NextRun:     nextRun(now, s.TargetDay),
		Courts:      courts,
	}
}

func (b *LiveBackend) Schedules() (SchedulesPayload, error) {
	schedules := b.cfg.GetSchedules()
	var sv []ScheduleView
	now := klNow()
	for _, s := range schedules {
		sv = append(sv, scheduleToView(s, now))
	}
	var accNames []string
	for _, a := range b.cfg.Accounts {
		accNames = append(accNames, a.Name)
	}
	return SchedulesPayload{Schedules: sv, Accounts: accNames, ScheduleFile: b.cfg.GetScheduleFile()}, nil
}

func scheduleFromRequest(req ScheduleRequest) (config.Schedule, error) {
	name := strings.TrimSpace(req.Name)
	day := strings.ToLower(strings.TrimSpace(req.TargetDay))
	var plan []config.BookingEntry
	for _, e := range req.BookingPlan {
		slot := strings.TrimSpace(e.Slot)
		var courts []string
		for _, c := range e.Courts {
			c = strings.TrimSpace(c)
			if c != "" {
				courts = append(courts, c)
			}
		}
		if slot == "" && len(courts) == 0 {
			continue
		}
		plan = append(plan, config.BookingEntry{Slot: slot, Courts: courts})
	}
	var accs []string
	for _, a := range req.Accounts {
		a = strings.TrimSpace(a)
		if a != "" {
			accs = append(accs, a)
		}
	}
	s := config.Schedule{Name: name, TargetDay: day, BookingPlan: plan, AccountNames: accs}
	if err := config.ValidateSchedule(s, nil); err != nil {
		// fallback validate with accounts if nil failed on unknown account check; will re-check with real accounts
	}
	return s, nil
}

func (b *LiveBackend) CreateSchedule(req ScheduleRequest) (ScheduleView, error) {
	s, _ := scheduleFromRequest(req)
	if err := config.ValidateSchedule(s, b.cfg.Accounts); err != nil {
		return ScheduleView{}, err
	}
	for _, exist := range b.cfg.GetSchedules() {
		if exist.Name == s.Name {
			return ScheduleView{}, fmt.Errorf("schedule %q already exists", s.Name)
		}
	}
	path := b.cfg.EffectiveScheduleFile()
	existing := b.cfg.GetSchedules()
	existing = append(existing, s)
	if err := config.SaveSchedulesFile(path, existing, b.cfg.Accounts); err != nil {
		return ScheduleView{}, err
	}
	b.cfg.SetSchedules(existing, path)
	log.Printf("schedule created: name=%s file=%s", s.Name, path)
	return scheduleToView(s, klNow()), nil
}

func (b *LiveBackend) UpdateSchedule(name string, req ScheduleRequest) (ScheduleView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ScheduleView{}, fmt.Errorf("schedule name required")
	}
	s, _ := scheduleFromRequest(req)
	// allow rename: if req.Name differs, treat as new name
	if s.Name == "" {
		s.Name = name
	}
	if err := config.ValidateSchedule(s, b.cfg.Accounts); err != nil {
		return ScheduleView{}, err
	}
	existing := b.cfg.GetSchedules()
	found := false
	for i, cur := range existing {
		if cur.Name == name {
			// if renaming, check duplicate
			if s.Name != name {
				for _, other := range existing {
					if other.Name == s.Name {
						return ScheduleView{}, fmt.Errorf("schedule %q already exists", s.Name)
					}
				}
			}
			existing[i] = s
			found = true
			break
		}
	}
	if !found {
		return ScheduleView{}, fmt.Errorf("unknown schedule %q", name)
	}
	path := b.cfg.EffectiveScheduleFile()
	if err := config.SaveSchedulesFile(path, existing, b.cfg.Accounts); err != nil {
		return ScheduleView{}, err
	}
	b.cfg.SetSchedules(existing, path)
	log.Printf("schedule updated: %s -> %s file=%s", name, s.Name, path)
	return scheduleToView(s, klNow()), nil
}

func (b *LiveBackend) DeleteSchedule(name string) error {
	name = strings.TrimSpace(name)
	existing := b.cfg.GetSchedules()
	var out []config.Schedule
	found := false
	for _, s := range existing {
		if s.Name == name {
			found = true
			continue
		}
		out = append(out, s)
	}
	if !found {
		return fmt.Errorf("unknown schedule %q", name)
	}
	path := b.cfg.EffectiveScheduleFile()
	if err := config.SaveSchedulesFile(path, out, b.cfg.Accounts); err != nil {
		return err
	}
	b.cfg.SetSchedules(out, path)
	log.Printf("schedule deleted: name=%s file=%s", name, path)
	return nil
}

// nextRun returns the next midnight (KL) whose weekday matches targetDay,
// mirroring the cron snipe schedule. Empty string if day unparseable.
func nextRun(now time.Time, targetDay string) string {
	days := map[string]time.Weekday{
		"sunday": time.Sunday, "sun": time.Sunday,
		"monday": time.Monday, "mon": time.Monday,
		"tuesday": time.Tuesday, "tue": time.Tuesday,
		"wednesday": time.Wednesday, "wed": time.Wednesday,
		"thursday": time.Thursday, "thu": time.Thursday,
		"friday": time.Friday, "fri": time.Friday,
		"saturday": time.Saturday, "sat": time.Saturday,
	}
	want, ok := days[strings.ToLower(strings.TrimSpace(targetDay))]
	if !ok {
		return ""
	}
	d := now
	for d.Weekday() != want {
		d = d.AddDate(0, 0, 1)
	}
	midnight := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
	if midnight.Before(now) {
		midnight = midnight.AddDate(0, 0, 7)
	}
	return midnight.Format("Mon Jan 2, 15:04")
}

func join(ss []string, sep string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}

// titleDay capitalizes the day name for display ("friday" -> "Friday").
func titleDay(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

func (b *LiveBackend) loginClient(email, password string) (*api.Client, error) {
	c := api.NewClient(b.cfg.BaseURL)
	if err := c.Login(email, password); err != nil {
		return nil, err
	}
	return c, nil
}

func (b *LiveBackend) Facilities() ([]FacilityView, error) {
	acc, err := b.primaryAccount()
	if err != nil {
		return nil, err
	}
	c, err := b.loginClient(acc.Email, acc.Password)
	if err != nil {
		return nil, err
	}
	fac, err := c.GetFacilities()
	if err != nil {
		return nil, err
	}
	var out []FacilityView
	for _, f := range fac {
		out = append(out, FacilityView{ID: f.ID, Name: f.Name})
	}
	return out, nil
}

func (b *LiveBackend) Bookings() ([]AccountBookings, error) {
	var out []AccountBookings
	for _, acc := range b.cfg.Accounts {
		c, err := b.loginClient(acc.Email, acc.Password)
		if err != nil {
			out = append(out, AccountBookings{Account: acc.Name, Error: err.Error()})
			continue
		}
		bl, err := c.GetBookings()
		if err != nil {
			out = append(out, AccountBookings{Account: acc.Name, Error: err.Error()})
			continue
		}
		ab := AccountBookings{Account: acc.Name}
		for _, x := range bl {
			ab.Bookings = append(ab.Bookings, BookingView{
				Date:     x.Date,
				Time:     shortTime(x.TimeStart) + "-" + shortTime(x.TimeEnd),
				Facility: x.Facility,
				Status:   x.Status,
			})
		}
		out = append(out, ab)
	}
	return out, nil
}

func shortTime(t string) string {
	if len(t) >= 5 {
		return t[:5]
	}
	return t
}

// resolveCourt matches api.Client.ResolveCourtNameToID semantics against an
// already-fetched facility list, avoiding one HTTP fetch per court.
// exactOnly skips the partial-match fallback (used for live booking where a
// typo must never silently pick the wrong court).
func resolveCourt(fac []api.Facility, input string, exactOnly bool) (string, error) {
	if isNumeric(input) {
		return input, nil
	}
	lower := strings.ToLower(strings.TrimSpace(input))
	for _, f := range fac {
		if strings.ToLower(f.Name) == lower {
			return f.ID, nil
		}
	}
	if exactOnly {
		return "", fmt.Errorf("court name %q not found (exact match required)", input)
	}
	for _, f := range fac {
		name := strings.ToLower(f.Name)
		if lower != "" && (strings.Contains(name, lower) || strings.Contains(lower, name)) {
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("court name %q not found", input)
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (b *LiveBackend) Probe(date string, courts []string) (ProbeResult, error) {
	acc, err := b.primaryAccount()
	if err != nil {
		return ProbeResult{}, err
	}
	c, err := b.loginClient(acc.Email, acc.Password)
	if err != nil {
		return ProbeResult{}, err
	}
	if len(courts) == 0 {
		courts = append(courts, b.cfg.FacilityIDs...)
	}
	fac, err := c.GetFacilities()
	if err != nil {
		return ProbeResult{}, err
	}
	res := ProbeResult{Date: date}
	for _, court := range courts {
		rid, err := resolveCourt(fac, court, false)
		if err != nil {
			res.Courts = append(res.Courts, CourtSlots{ID: court, Name: court, Error: err.Error()})
			continue
		}
		slots, err := c.GetTimeslots(rid, date)
		if err != nil {
			res.Courts = append(res.Courts, CourtSlots{ID: rid, Name: court, Error: err.Error()})
			continue
		}
		cs := CourtSlots{ID: rid, Name: court}
		for _, s := range slots {
			cs.Slots = append(cs.Slots, SlotView{Time: s.Time, Available: s.Available})
		}
		res.Courts = append(res.Courts, cs)
	}
	return res, nil
}

func (b *LiveBackend) Book(req BookRequest) (BookResponse, error) {
	acc, err := b.primaryAccount()
	if err != nil {
		return BookResponse{}, err
	}
	courts := []string{req.FacilityID}
	if req.FacilityID == "" {
		courts = append([]string{}, b.cfg.FacilityIDs...)
	}
	if len(courts) == 0 {
		return BookResponse{}, ErrNoCourts
	}
	c, err := b.loginClient(acc.Email, acc.Password)
	if err != nil {
		return BookResponse{}, err
	}
	fac, err := c.GetFacilities()
	if err != nil {
		return BookResponse{}, err
	}
	var target string
	failures := 0
	for _, court := range courts {
		rid, err := resolveCourt(fac, court, true)
		if err != nil {
			failures++
			continue
		}
		slots, err := c.GetTimeslots(rid, req.Date)
		if err != nil {
			failures++
			continue
		}
		for _, s := range slots {
			if s.Time == req.Time && s.Available {
				target = rid
				break
			}
		}
		if target != "" {
			break
		}
	}
	if target == "" {
		if failures == len(courts) {
			return BookResponse{}, fmt.Errorf("could not check availability: all %d court lookups failed", len(courts))
		}
		log.Printf("UI book: date=%s time=%s dryRun=%v result=no-availability", req.Date, req.Time, req.DryRun)
		return BookResponse{DryRun: req.DryRun, Message: fmt.Sprintf("%s not available on tried courts for %s", req.Time, req.Date)}, nil
	}
	if req.DryRun {
		log.Printf("UI book dry-run: date=%s time=%s court=%s", req.Date, req.Time, target)
		return BookResponse{DryRun: true, Court: target, Message: fmt.Sprintf("Would book court %s, %s on %s", target, req.Time, req.Date)}, nil
	}
	result, err := c.BookSlot(target, acc.UnitID, acc.BookingName, acc.Contact, req.Date, req.Time)
	if err != nil {
		log.Printf("UI book error: date=%s time=%s court=%s err=%v", req.Date, req.Time, target, err)
		return BookResponse{}, err
	}
	log.Printf("UI book live: date=%s time=%s court=%s status=%v insertID=%d", req.Date, req.Time, target, result.Status, result.InsertID)
	if result.Status {
		return BookResponse{Court: target, Booked: true, InsertID: result.InsertID, Message: result.Msg}, nil
	}
	return BookResponse{Court: target, Message: result.MsgTitle + " - " + result.Msg}, nil
}
