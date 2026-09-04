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
	return StatusPayload{
		TargetDay:   titleDay(b.cfg.TargetDay),
		TargetDate:  targetDate,
		NextRun:     nextRun(now, b.cfg.TargetDay),
		Accounts:    accounts,
		BookingPlan: plan,
	}
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
func resolveCourt(fac []api.Facility, input string) (string, error) {
	if isNumeric(input) {
		return input, nil
	}
	lower := strings.ToLower(strings.TrimSpace(input))
	for _, f := range fac {
		if strings.ToLower(f.Name) == lower {
			return f.ID, nil
		}
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
		rid, err := resolveCourt(fac, court)
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
	c, err := b.loginClient(acc.Email, acc.Password)
	if err != nil {
		return BookResponse{}, err
	}
	courts := []string{req.FacilityID}
	if req.FacilityID == "" {
		courts = append([]string{}, b.cfg.FacilityIDs...)
	}
	fac, err := c.GetFacilities()
	if err != nil {
		return BookResponse{}, err
	}
	var target string
	failures := 0
	for _, court := range courts {
		rid, err := resolveCourt(fac, court)
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
