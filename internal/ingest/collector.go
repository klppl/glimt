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
}

func NewCollector(reg *sites.Registry, in *Ingestor, realIPHeader string) *Collector {
	return &Collector{reg: reg, in: in, realIPHeader: realIPHeader}
}

func (c *Collector) Handle(w http.ResponseWriter, r *http.Request) {
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
	ev := c.in.build(site, p, ip, r.UserAgent(), r.Header.Get("Accept-Language"))
	c.in.Enqueue(ev)

	w.WriteHeader(http.StatusNoContent)
}

// clientIP reads the configured real-IP header (first hop) when present, else
// falls back to the connection's remote address. The value is used transiently.
func (c *Collector) clientIP(r *http.Request) string {
	if c.realIPHeader != "" {
		if v := r.Header.Get(c.realIPHeader); v != "" {
			if i := strings.IndexByte(v, ','); i >= 0 {
				v = v[:i]
			}
			return strings.TrimSpace(v)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
