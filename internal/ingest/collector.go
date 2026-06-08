package ingest

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/klppl/glimt/internal/sites"
)

const maxBody = 16 << 10 // 16 KiB cap on event payloads

// Collector is the same-origin ingest endpoint. It validates the collect token,
// enriches the event (consuming the IP), enqueues it, and returns 204 fast.
type Collector struct {
	reg          *sites.Registry
	in           *Ingestor
	realIPHeader string
	trustedNets  []*net.IPNet
	cfCountry    bool
}

func NewCollector(reg *sites.Registry, in *Ingestor, realIPHeader string, trustedNets []*net.IPNet, cfCountry bool) *Collector {
	return &Collector{
		reg:          reg,
		in:           in,
		realIPHeader: realIPHeader,
		trustedNets:  trustedNets,
		cfCountry:    cfCountry,
	}
}

// Preflight answers CORS preflight (only sent if a client uses a non-simple
// request; normal sendBeacon/fetch text/plain posts skip it).
func (c *Collector) Preflight(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

func (c *Collector) Handle(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)

	tok := chi.URLParam(r, "token")
	site, ok := c.reg.ByCollect(tok)
	if !ok {
		// Look like an ordinary, content-free endpoint to anyone probing.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ip := c.clientIP(r)
	ev := c.in.build(site, p, ip, r.UserAgent(), r.Header.Get("Accept-Language"), c.country(r))
	c.in.Enqueue(ev)

	w.WriteHeader(http.StatusNoContent)
}

// clientIP returns the connecting peer's address, or the configured real-IP
// header when the peer is a trusted proxy. With no trusted proxies configured
// the header is honored unconditionally (single-host / dev). The value is used
// transiently for the daily hash + geo and is never stored.
func (c *Collector) clientIP(r *http.Request) string {
	peer := peerIP(r.RemoteAddr)
	if c.realIPHeader != "" && c.trustPeer(peer) {
		if v := r.Header.Get(c.realIPHeader); v != "" {
			if i := strings.IndexByte(v, ','); i >= 0 {
				v = v[:i]
			}
			return strings.TrimSpace(v)
		}
	}
	return peer
}

func (c *Collector) trustPeer(ip string) bool {
	if len(c.trustedNets) == 0 {
		return true
	}
	pip := net.ParseIP(ip)
	if pip == nil {
		return false
	}
	for _, n := range c.trustedNets {
		if n.Contains(pip) {
			return true
		}
	}
	return false
}

// country reads Cloudflare's CF-IPCountry header when enabled, filtering the
// non-country sentinels CF uses (XX = unknown, T1 = Tor). Returns "" otherwise,
// in which case the local GeoIP database (if any) is consulted instead.
func (c *Collector) country(r *http.Request) string {
	if !c.cfCountry {
		return ""
	}
	cc := r.Header.Get("CF-IPCountry")
	if len(cc) != 2 || cc == "XX" || cc == "T1" {
		return ""
	}
	return cc
}

// corsHeaders allows the cross-origin POST when the script is served from a
// different origin than the tracked page (e.g. a dedicated analytics host). The
// response carries no data, so a wildcard origin is safe and no credentials are
// used.
func corsHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type")
	h.Set("Access-Control-Max-Age", "86400")
}

func peerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
