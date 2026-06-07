// Package tracker serves the tiny, first-party tracking snippet from a
// per-site randomized path. The snippet carries no recognizable names and posts
// to the same-origin collect endpoint, so no ad-block filter list can match it.
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
}

func New(reg *sites.Registry, jsGlobal string) *Handler {
	if jsGlobal == "" {
		jsGlobal = "glimt"
	}
	return &Handler{reg: reg, jsGlobal: jsGlobal}
}

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	tok := chi.URLParam(r, "token")
	site, ok := h.reg.ByScript(tok)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(Render(site.CollectToken, h.jsGlobal)))
}

// Render returns the snippet JS with the collect endpoint and global name
// substituted. Used both when serving the script and for install instructions.
func Render(collectToken, jsGlobal string) string {
	if jsGlobal == "" {
		jsGlobal = "glimt"
	}
	return strings.NewReplacer(
		"__EP__", "/e/"+collectToken,
		"__G__", jsGlobal,
	).Replace(snippet)
}
