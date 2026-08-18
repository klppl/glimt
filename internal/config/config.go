// Package config loads glimt configuration from an optional JSON file and
// environment variables. Env vars (prefixed GLIMT_) always win over the file.
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr            string `json:"addr"`              // listen address, e.g. ":8080"
	DBPath          string `json:"db_path"`           // SQLite file path
	GeoDBPath       string `json:"geo_db_path"`       // optional mmdb (DB-IP or GeoLite2)
	AdminUser       string `json:"admin_user"`        // bootstrap admin username
	AdminPass       string `json:"admin_pass"`        // bootstrap admin password
	BaseURL         string `json:"base_url"`          // public base URL (for cookie Secure + share links)
	RealIPHeader    string `json:"real_ip_header"`    // header carrying the client IP behind your proxy
	TrustedProxies  string `json:"trusted_proxies"`   // CIDRs / "cloudflare" / "private" allowed to set the real-IP header
	CFCountry       bool   `json:"cf_country"`        // derive country from Cloudflare's CF-IPCountry header
	JSGlobal        string `json:"js_global"`         // global name the snippet exposes for custom events
	SessionTTLHours int    `json:"session_ttl_hours"` // admin login session lifetime
	RetentionDays   int    `json:"retention_days"`    // raw event retention in days (0 = keep forever)

	SessionTTL       time.Duration `json:"-"`
	TrustedProxyNets []*net.IPNet  `json:"-"`
}

func Load() (*Config, error) {
	c := &Config{
		Addr:            ":8080",
		DBPath:          "glimt.db",
		RealIPHeader:    "X-Forwarded-For",
		JSGlobal:        "glimt",
		SessionTTLHours: 24 * 7,
		RetentionDays:   0,
	}

	if p := os.Getenv("GLIMT_CONFIG"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, c); err != nil {
			return nil, err
		}
	}

	envStr("GLIMT_ADDR", &c.Addr)
	envStr("GLIMT_DB", &c.DBPath)
	envStr("GLIMT_GEO_DB", &c.GeoDBPath)
	envStr("GLIMT_ADMIN_USER", &c.AdminUser)
	envStr("GLIMT_ADMIN_PASS", &c.AdminPass)
	envStr("GLIMT_BASE_URL", &c.BaseURL)
	envStr("GLIMT_REAL_IP_HEADER", &c.RealIPHeader)
	envStr("GLIMT_TRUSTED_PROXIES", &c.TrustedProxies)
	envStr("GLIMT_JS_GLOBAL", &c.JSGlobal)
	if v := os.Getenv("GLIMT_CF_COUNTRY"); v != "" {
		c.CFCountry = boolVal(v)
	}
	if v := os.Getenv("GLIMT_SESSION_TTL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.SessionTTLHours = n
		}
	}
	if v := os.Getenv("GLIMT_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.RetentionDays = n
		}
	}

	c.SessionTTL = time.Duration(c.SessionTTLHours) * time.Hour
	c.TrustedProxyNets = parseTrustedProxies(c.TrustedProxies)
	return c, nil
}

func envStr(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func boolVal(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// parseTrustedProxies turns a comma-separated list of CIDRs, bare IPs, and the
// keywords "cloudflare" / "private" into a set of networks. A request's real-IP
// header is honored only when the connecting peer falls inside one of these.
func parseTrustedProxies(raw string) []*net.IPNet {
	var nets []*net.IPNet
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch strings.ToLower(part) {
		case "cloudflare":
			nets = append(nets, mustCIDRs(cloudflareCIDRs)...)
		case "private":
			nets = append(nets, mustCIDRs(privateCIDRs)...)
		default:
			if _, n, err := net.ParseCIDR(part); err == nil {
				nets = append(nets, n)
			} else if ip := net.ParseIP(part); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				if _, n, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), bits)); err == nil {
					nets = append(nets, n)
				}
			}
		}
	}
	return nets
}

func mustCIDRs(list []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(list))
	for _, c := range list {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// Cloudflare's published edge ranges (https://www.cloudflare.com/ips/).
var cloudflareCIDRs = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
}

// Private / loopback / link-local ranges — useful when a local reverse proxy
// (e.g. npmplus in Docker) sits between Cloudflare and glimt.
var privateCIDRs = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16",
	"::1/128", "fc00::/7", "fe80::/10",
}
