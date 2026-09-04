package api

import (
	"net/http"
	"testing"
	"time"
)

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
