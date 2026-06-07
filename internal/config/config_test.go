package config

import (
	"net"
	"testing"
)

func contains(nets []*net.IPNet, ip string) bool {
	p := net.ParseIP(ip)
	for _, n := range nets {
		if n.Contains(p) {
			return true
		}
	}
	return false
}

func TestParseTrustedProxies(t *testing.T) {
	nets := parseTrustedProxies("cloudflare, private, 203.0.113.0/24, 198.51.100.9")

	cases := map[string]bool{
		"104.16.5.5":   true,  // cloudflare range
		"2606:4700::1": true,  // cloudflare v6
		"10.0.0.5":     true,  // private
		"127.0.0.1":    true,  // loopback (private)
		"203.0.113.7":  true,  // explicit CIDR
		"198.51.100.9": true,  // bare IP -> /32
		"198.51.100.8": false, // adjacent to bare IP, not included
		"8.8.8.8":      false, // public, untrusted
	}
	for ip, want := range cases {
		if got := contains(nets, ip); got != want {
			t.Errorf("contains(%s) = %v, want %v", ip, got, want)
		}
	}

	if n := parseTrustedProxies(""); len(n) != 0 {
		t.Errorf("empty config = %d nets, want 0", len(n))
	}
}
