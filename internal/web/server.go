package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Payload types shared by stub/live backends and API responses.

type AccountView struct {
	Name string   `json:"name"`
	Plan []string `json:"plan,omitempty"`
}

type StatusPayload struct {
	TargetDay   string        `json:"targetDay"`
	TargetDate  string        `json:"targetDate"`
	NextRun     string        `json:"nextRun,omitempty"`
	Accounts    []AccountView `json:"accounts"`
	BookingPlan []string      `json:"bookingPlan,omitempty"`
}

type FacilityView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BookingView struct {
	Date     string `json:"date"`
	Time     string `json:"time"`
	Facility string `json:"facility"`
	Status   string `json:"status"`
}

type AccountBookings struct {
	Account  string        `json:"account"`
	Bookings []BookingView `json:"bookings,omitempty"`
	Error    string        `json:"error,omitempty"`
}

type SlotView struct {
	Time      string `json:"time"`
	Available bool   `json:"available"`
}

type CourtSlots struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Slots []SlotView `json:"slots,omitempty"`
	Error string     `json:"error,omitempty"`
}

type ProbeResult struct {
	Date   string       `json:"date"`
	Courts []CourtSlots `json:"courts,omitempty"`
}

type BookRequest struct {
	Date       string `json:"date"`
	Time       string `json:"time"`
	FacilityID string `json:"facilityId,omitempty"`
	DryRun     bool   `json:"dryRun"`
	Confirm    bool   `json:"confirm,omitempty"`
}

type BookResponse struct {
	DryRun   bool   `json:"dryRun"`
	Booked   bool   `json:"booked,omitempty"`
	Court    string `json:"court,omitempty"`
	Message  string `json:"message"`
	InsertID int    `json:"insertId,omitempty"`
}

// Backend abstracts gprop access so handlers stay testable.
type Backend interface {
	Status() StatusPayload
	Facilities() ([]FacilityView, error)
	Bookings() ([]AccountBookings, error)
	Probe(date string, courts []string) (ProbeResult, error)
	Book(req BookRequest) (BookResponse, error)
}

type ServerOptions struct {
	Password string
	Backend  Backend
}

type session struct {
	csrf    string
	expires time.Time
}

type Server struct {
	password string
	backend  Backend
	mu       sync.Mutex
	sessions map[string]session
	fails    map[string]*failWindow
}

type failWindow struct {
	count int
	since time.Time
}

const (
	sessionCookie = "courtbot_session"
	sessionTTL    = 24 * time.Hour
)

func NewServer(opts ServerOptions) *Server {
	return &Server{
		password: opts.Password,
		backend:  opts.Backend,
		sessions: map[string]session{},
		fails:    map[string]*failWindow{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/status", s.requireAuth(s.handleStatus))
	mux.HandleFunc("/api/facilities", s.requireAuth(s.handleFacilities))
	mux.HandleFunc("/api/bookings", s.requireAuth(s.handleBookings))
	mux.HandleFunc("/api/probe", s.requireAuth(s.handleProbe))
	mux.HandleFunc("/api/book", s.requireAuth(s.requireCSRF(s.handleBook)))
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func (s *Server) tooManyFails(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	fw, ok := s.fails[ip]
	if !ok {
		return false
	}
	if time.Since(fw.since) > 5*time.Minute {
		delete(s.fails, ip)
		return false
	}
	return fw.count >= 5
}

func (s *Server) recordFail(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fw, ok := s.fails[ip]
	if !ok || time.Since(fw.since) > 5*time.Minute {
		s.fails[ip] = &failWindow{count: 1, since: time.Now()}
		return
	}
	fw.count++
}

func (s *Server) clearFails(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.fails, ip)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ip := clientIP(r)
	if s.tooManyFails(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many attempts, try later")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.password)) != 1 || s.password == "" {
		s.recordFail(ip)
		writeErr(w, http.StatusUnauthorized, "invalid password")
		return
	}
	s.clearFails(ip)
	tok := randomToken(32)
	csrf := randomToken(32)
	s.mu.Lock()
	s.sessions[tok] = session{csrf: csrf, expires: time.Now().Add(sessionTTL)}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]string{"csrfToken": csrf})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookie)
	if err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[c.Value]
	if !ok {
		return false
	}
	if time.Now().After(sess.expires) {
		delete(s.sessions, c.Value)
		return false
	}
	return true
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validSession(r) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) requireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next(w, r)
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		s.mu.Lock()
		sess, ok := s.sessions[c.Value]
		s.mu.Unlock()
		if !ok {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(sess.csrf)) != 1 {
			writeErr(w, http.StatusForbidden, "bad csrf token")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.backend.Status())
}

func (s *Server) handleFacilities(w http.ResponseWriter, _ *http.Request) {
	f, err := s.backend.Facilities()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"facilities": f})
}

func (s *Server) handleBookings(w http.ResponseWriter, _ *http.Request) {
	b, err := s.backend.Bookings()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": b})
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		writeErr(w, http.StatusBadRequest, "date required (YYYY-MM-DD)")
		return
	}
	var courts []string
	if c := strings.TrimSpace(r.URL.Query().Get("courts")); c != "" {
		for _, p := range strings.Split(c, ",") {
			if p = strings.TrimSpace(p); p != "" {
				courts = append(courts, p)
			}
		}
	}
	res, err := s.backend.Probe(date, courts)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleBook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req BookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Date = strings.TrimSpace(req.Date)
	req.Time = strings.TrimSpace(req.Time)
	if req.Date == "" || req.Time == "" {
		writeErr(w, http.StatusBadRequest, "date and time required")
		return
	}
	if !req.DryRun && !req.Confirm {
		writeErr(w, http.StatusBadRequest, "live booking requires confirm:true")
		return
	}
	res, err := s.backend.Book(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}
