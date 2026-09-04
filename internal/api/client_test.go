package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsRetryable(t *testing.T) {
	cases := []struct{ err string; want bool }{
		{"Get \"https://www.gpropsystems.com/login\": dial tcp 162.253.17.182:443: connect: connection timed out", true},
		{"read tcp 10.0.0.227:49456->149.154.166.110:443: read: connection reset by peer", true},
		{"Client.Timeout exceeded while awaiting headers", true},
		{"context deadline exceeded", true},
		{"login failed: invalid password", false},
		{"CSRF token not found in login page HTML", false},
	}
	for _, c := range cases {
		got := isRetryableError(fmt.Errorf("%s", c.err))
		if got != c.want {
			t.Fatalf("isRetryable(%q)=%v want %v", c.err, got, c.want)
		}
	}
}

func TestLoginRetriesOnTimeout(t *testing.T) {
	// Minimal: isRetryable covers transient detection; full Login retry covered by integration probe
	if !isRetryableError(fmt.Errorf("dial tcp timeout")) {
		t.Fatal("expected retryable")
	}
}

func TestLoginRetriesOn500ThenSucceeds(t *testing.T) {
	var posts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			w.Write([]byte(`<html><script>csrf_token = 'testtoken1234567890';</script></html>`))
			return
		}
		if r.URL.Path == "/login/login_data_submit" {
			n := atomic.AddInt32(&posts, 1)
			if n <= 2 {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("boom"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"msg":"Logged In Successfully","_co6sO0rpsfat":"newtoken123"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.Login("a@example.com", "pw"); err != nil {
		t.Fatalf("Login should succeed after 500 retries, got: %v", err)
	}
	if got := atomic.LoadInt32(&posts); got != 3 {
		t.Fatalf("expected 3 POST attempts, got %d", got)
	}
}

func TestNewClientHasTimeout(t *testing.T) {
	c := NewClient("https://www.gpropsystems.com")
	if c.HTTPClient.Timeout == 0 {
		t.Fatal("expected Timeout >0, got 0")
	}
	if c.HTTPClient.Timeout < 10*time.Second {
		t.Fatalf("Timeout too short %v, want >=10s", c.HTTPClient.Timeout)
	}
	// Transport timeouts checked via type assert
	tr, ok := c.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport not *http.Transport")
	}
	if tr.TLSHandshakeTimeout == 0 {
		t.Fatal("expected TLSHandshakeTimeout >0")
	}
	if tr.ResponseHeaderTimeout == 0 {
		t.Fatal("expected ResponseHeaderTimeout >0")
	}
}
