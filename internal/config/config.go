// Package config loads glimt configuration from an optional JSON file and
// environment variables. Env vars (prefixed GLIMT_) always win over the file.
package config

import (
	"encoding/json"
	"os"
	"strconv"
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
	JSGlobal        string `json:"js_global"`         // global name the snippet exposes for custom events
	SessionTTLHours int    `json:"session_ttl_hours"` // admin login session lifetime

	SessionTTL time.Duration `json:"-"`
}

func Load() (*Config, error) {
	c := &Config{
		Addr:            ":8080",
		DBPath:          "glimt.db",
		RealIPHeader:    "X-Forwarded-For",
		JSGlobal:        "glimt",
		SessionTTLHours: 24 * 7,
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
	envStr("GLIMT_JS_GLOBAL", &c.JSGlobal)
	if v := os.Getenv("GLIMT_SESSION_TTL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.SessionTTLHours = n
		}
	}

	c.SessionTTL = time.Duration(c.SessionTTLHours) * time.Hour
	return c, nil
}

func envStr(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}
