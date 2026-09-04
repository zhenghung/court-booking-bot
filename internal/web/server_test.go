package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer() *Server {
	return NewServer(ServerOptions{
		Password: "test-secret",
		Backend:  stubBackend{},
	})
}

type stubBackend struct{}

func (stubBackend) Status() StatusPayload {
	return StatusPayload{TargetDay: "friday", TargetDate: "2026-09-11", Accounts: []AccountView{{Name: "Primary"}}}
}
func (stubBackend) Facilities() ([]FacilityView, error) { return []FacilityView{{ID: "7935", Name: "P1"}}, nil }
func (stubBackend) Bookings() ([]AccountBookings, error) {
	return []AccountBookings{{Account: "Primary"}}, nil
}
func (stubBackend) Probe(date string, courts []string) (ProbeResult, error) {
	return ProbeResult{Date: date}, nil
}
func (stubBackend) Book(BookRequest) (BookResponse, error) {
	return BookResponse{DryRun: true, Message: "Would book"}, nil
}

func TestHealthzNoAuth(t *testing.T) {
	s := newTestServer()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", w.Code)
	}
}

func TestIndexPublicNoAuth(t *testing.T) {
	s := newTestServer()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("index unauth = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("index content-type = %q, want text/html", ct)
	}
}

func TestStatusRequiresAuth(t *testing.T) {
	s := newTestServer()
	r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status unauth = %d, want 401", w.Code)
	}
}

func TestLogoutRequiresPost(t *testing.T) {
	s := newTestServer()
	r := httptest.NewRequest(http.MethodGet, "/api/logout", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("logout GET = %d, want 405", w.Code)
	}
}

func loginCookies(t *testing.T, s *Server) ([]*http.Cookie, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"test-secret"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200", w.Code)
	}
	var lr struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.NewDecoder(w.Result().Body).Decode(&lr); err != nil || lr.CSRF == "" {
		t.Fatalf("login missing csrf: %v", err)
	}
	return w.Result().Cookies(), lr.CSRF
}

func TestProbeRejectsBadDate(t *testing.T) {
	s := newTestServer()
	cookies, _ := loginCookies(t, s)
	r := httptest.NewRequest(http.MethodGet, "/api/probe?date=not-a-date", nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("probe bad date = %d, want 400", w.Code)
	}
}

func TestBookRejectsBadDate(t *testing.T) {
	s := newTestServer()
	cookies, csrf := loginCookies(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/book", strings.NewReader(`{"date":"11-09-2026","time":"07:00-08:00","dryRun":true}`))
	for _, c := range cookies {
		r.AddCookie(c)
	}
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("book bad date = %d, want 400", w.Code)
	}
}

func TestSessionResume(t *testing.T) {
	s := newTestServer()
	// unauth -> 401
	r := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("session unauth = %d, want 401", w.Code)
	}
	// authed -> csrf matching login
	cookies, csrf := loginCookies(t, s)
	r2 := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	for _, c := range cookies {
		r2.AddCookie(c)
	}
	w2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("session authed = %d, want 200", w2.Code)
	}
	var body struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.NewDecoder(w2.Result().Body).Decode(&body); err != nil || body.CSRF != csrf {
		t.Fatalf("session csrf mismatch: %v", err)
	}
}

func TestBookRejectsBadTime(t *testing.T) {
	s := newTestServer()
	cookies, csrf := loginCookies(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/book", strings.NewReader(`{"date":"2026-09-12","time":"whenever","dryRun":true}`))
	for _, c := range cookies {
		r.AddCookie(c)
	}
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("book bad time = %d, want 400", w.Code)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	s := newTestServer()
	r := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"nope"}`))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("login wrong = %d, want 401", w.Code)
	}
}

func TestLoginThenStatusAndCSRFEnforced(t *testing.T) {
	s := newTestServer()
	h := s.Handler()

	// login
	r := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"test-secret"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200", w.Code)
	}
	var lr struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.NewDecoder(w.Result().Body).Decode(&lr); err != nil || lr.CSRF == "" {
		t.Fatalf("login missing csrf token: %v", err)
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login missing session cookie")
	}

	// authed GET works
	r2 := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	for _, c := range cookies {
		r2.AddCookie(c)
	}
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("status authed = %d, want 200", w2.Code)
	}

	// POST without CSRF rejected
	r3 := httptest.NewRequest(http.MethodPost, "/api/book", strings.NewReader(`{"date":"2026-09-11","time":"07:00-08:00","dryRun":true}`))
	for _, c := range cookies {
		r3.AddCookie(c)
	}
	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, r3)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("book without csrf = %d, want 403", w3.Code)
	}

	// POST with CSRF passes
	r4 := httptest.NewRequest(http.MethodPost, "/api/book", strings.NewReader(`{"date":"2026-09-11","time":"07:00-08:00","dryRun":true}`))
	for _, c := range cookies {
		r4.AddCookie(c)
	}
	r4.Header.Set("X-CSRF-Token", lr.CSRF)
	w4 := httptest.NewRecorder()
	h.ServeHTTP(w4, r4)
	if w4.Code != http.StatusOK {
		t.Fatalf("book with csrf = %d, want 200", w4.Code)
	}
}
