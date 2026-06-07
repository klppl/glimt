// Package geo resolves an IP to country/region in-memory from a local mmdb file.
// It reads both DB-IP and MaxMind GeoLite2 databases (same on-disk format).
// When no database is configured it degrades to empty results.
package geo

import (
	"net"

	"github.com/oschwald/maxminddb-golang"
)

type Geo struct {
	r *maxminddb.Reader
}

// Open loads the mmdb at path. An empty path returns a no-op resolver.
func Open(path string) (*Geo, error) {
	if path == "" {
		return &Geo{}, nil
	}
	r, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	return &Geo{r: r}, nil
}

func (g *Geo) Close() error {
	if g.r != nil {
		return g.r.Close()
	}
	return nil
}

func (g *Geo) Enabled() bool { return g.r != nil }

// record matches the common subset of DB-IP and GeoLite2 City/Country schemas.
type record struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	Subdivisions []struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"subdivisions"`
}

// Lookup returns ISO country and first subdivision (region) codes. The IP is
// never stored by callers; it lives only for the duration of this call.
func (g *Geo) Lookup(ipStr string) (country, region string) {
	if g.r == nil || ipStr == "" {
		return "", ""
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", ""
	}
	var rec record
	if err := g.r.Lookup(ip, &rec); err != nil {
		return "", ""
	}
	country = rec.Country.ISOCode
	if len(rec.Subdivisions) > 0 {
		region = rec.Subdivisions[0].ISOCode
	}
	return country, region
}
