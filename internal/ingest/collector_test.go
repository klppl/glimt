package ingest

import (
	"net"
	"net/http/httptest"
	"testing"
)

func mustNets(t *testing.T, cidrs ...string) []*net.IPNet {
	t.Helper()
	var out []*net.IPNet
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

func TestClientIPTrust(t *testing.T) {
	// Header is honored only when the connecting peer is a trusted proxy.
	c := &Collector{realIPHeader: "X-Forwarded-For", trustedNets: mustNets(t, "10.0.0.0/8")}

	r := httptest.NewRequest("POST", "/e/x", nil)
	r.RemoteAddr = "10.1.2.3:5000" // trusted peer
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.1.2.3")
	if got := c.clientIP(r); got != "203.0.113.9" {
		t.Errorf("trusted peer: clientIP = %q, want 203.0.113.9", got)
	}

	r2 := httptest.NewRequest("POST", "/e/x", nil)
	r2.RemoteAddr = "8.8.8.8:5000" // untrusted peer trying to spoof
	r2.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := c.clientIP(r2); got != "8.8.8.8" {
		t.Errorf("untrusted peer: clientIP = %q, want 8.8.8.8 (header ignored)", got)
	}
}

func TestClientIPNoTrustedProxies(t *testing.T) {
	// Backward-compatible: with no trusted proxies, the header is honored.
	c := &Collector{realIPHeader: "X-Forwarded-For"}
	r := httptest.NewRequest("POST", "/e/x", nil)
	r.RemoteAddr = "8.8.8.8:5000"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := c.clientIP(r); got != "203.0.113.9" {
		t.Errorf("clientIP = %q, want 203.0.113.9", got)
	}
}

func TestCFCountry(t *testing.T) {
	c := &Collector{cfCountry: true}
	r := httptest.NewRequest("POST", "/e/x", nil)
	r.Header.Set("CF-IPCountry", "SE")
	if got := c.country(r); got != "SE" {
		t.Errorf("country = %q, want SE", got)
	}
	for _, sentinel := range []string{"XX", "T1", ""} {
		r.Header.Set("CF-IPCountry", sentinel)
		if got := c.country(r); got != "" {
			t.Errorf("country(%q) = %q, want empty", sentinel, got)
		}
	}
	// Disabled => never reads the header.
	off := &Collector{cfCountry: false}
	r.Header.Set("CF-IPCountry", "SE")
	if got := off.country(r); got != "" {
		t.Errorf("disabled country = %q, want empty", got)
	}
}
