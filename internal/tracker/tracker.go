// Package tracker serves the tiny, first-party tracking snippet from a
// per-site randomized path. The snippet carries no recognizable names. Its
// collect endpoint is baked as an absolute URL pointing back at glimt, so it
// works whether the script is served from the site's own domain (true
// first-party via a reverse proxy) or from a dedicated analytics host.
package tracker

import (
	_ "embed"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/klppl/glimt/internal/sites"
)

//go:embed snippet.js
var snippet string

type Handler struct {
	reg      *sites.Registry
	jsGlobal string
	baseURL  string // configured public base, e.g. https://stats.example.com ("" => derive from request)
}

func New(reg *sites.Registry, jsGlobal, baseURL string) *Handler {
	if jsGlobal == "" {
		jsGlobal = "glimt"
	}
	return &Handler{reg: reg, jsGlobal: jsGlobal, baseURL: strings.TrimRight(baseURL, "/")}
}

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	tok := chi.URLParam(r, "token")
	site, ok := h.reg.ByScript(tok)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	// Modest cache: snippet updates (and path rotations) propagate within ~an
	// hour rather than a day, while still sparing the origin most hits.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(Render(site.CollectToken, h.jsGlobal, h.endpointBase(r))))
}

// endpointBase resolves the origin the beacon should target. Prefer the
// configured base URL; otherwise derive it from the host the script was fetched
// from (protocol-relative so it matches the page's scheme).
func (h *Handler) endpointBase(r *http.Request) string {
	if h.baseURL != "" {
		return h.baseURL
	}
	if r.Host != "" {
		return "//" + r.Host
	}
	return ""
}

// Render returns the snippet JS with the collect token, fallback endpoint, and
// global name substituted. At runtime the snippet first tries to locate its
// endpoint relative to its own <script src> (so a first-party reverse proxy
// "just works"); __EP__ is the absolute fallback used when that isn't possible.
// base is a URL origin (e.g. "https://stats.example.com" or "//stats.example.com");
// empty yields a same-origin relative path.
func Render(collectToken, jsGlobal, base string) string {
	if jsGlobal == "" {
		jsGlobal = "glimt"
	}
	return strings.NewReplacer(
		"__CT__", collectToken,
		"__EP__", base+"/e/"+collectToken,
		"__G__", jsGlobal,
	).Replace(snippet)
}
