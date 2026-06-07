// Package dashboard renders the server-side, HTMX-driven admin UI and the
// public share view. Charts are inline SVG; top-N panels are HTML bars.
package dashboard

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/klppl/glimt/internal/auth"
	"github.com/klppl/glimt/internal/model"
	"github.com/klppl/glimt/internal/query"
	"github.com/klppl/glimt/internal/sites"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Config struct {
	JSGlobal   string
	BaseURL    string
	GeoEnabled bool
}

type Handlers struct {
	reg   *sites.Registry
	q     *query.Querier
	auth  *auth.Auth
	cfg   Config
	pages map[string]*template.Template
}

func New(reg *sites.Registry, q *query.Querier, a *auth.Auth, cfg Config) (*Handlers, error) {
	h := &Handlers{reg: reg, q: q, auth: a, cfg: cfg}
	if err := h.parseTemplates(); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *Handlers) funcMap() template.FuncMap {
	return template.FuncMap{
		"num":     fmtNum,
		"pct":     fmtPct,
		"dur":     fmtDuration,
		"flag":    flag,
		"country": countryName,
	}
}

func (h *Handlers) parseTemplates() error {
	h.pages = map[string]*template.Template{}
	for _, page := range []string{"index", "login", "settings"} {
		t, err := template.New("base.html").Funcs(h.funcMap()).
			ParseFS(templatesFS, "templates/base.html", "templates/"+page+".html")
		if err != nil {
			return err
		}
		h.pages[page] = t
	}
	return nil
}

func (h *Handlers) render(w http.ResponseWriter, page string, data *viewData) {
	t, ok := h.pages[page]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("dashboard: render %s: %v", page, err)
	}
}

// ---- view model ----

type rangeOpt struct {
	Key, Label string
	Selected   bool
}

type barRow struct {
	Display  string
	Count    int
	Visitors int
	Percent  float64
}

type panel struct {
	Title  string
	Metric string // column label for the right-hand number
	Rows   []barRow
}

type siteSetting struct {
	Site      *model.Site
	ScriptURL string
	Snippet   string
	ShareURL  string
}

type viewData struct {
	Title      string
	Sites      []*model.Site
	Site       *model.Site
	SiteRows   []siteSetting
	Range      string
	Ranges     []rangeOpt
	Summary    query.Summary
	Chart      template.HTML
	Panels     []panel
	Realtime   int
	GeoEnabled bool
	Public     bool
	ShareToken string
	NewAPIKey  string
	BaseURL    string
	Error      string
}

// ---- range handling ----

func resolveRange(key string) (fromMs, toMs int64, interval, norm string) {
	now := time.Now().UTC()
	toMs = now.UnixMilli()
	switch key {
	case "today":
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return from.UnixMilli(), toMs, "hour", "today"
	case "24h":
		return now.Add(-24 * time.Hour).UnixMilli(), toMs, "hour", "24h"
	case "30d":
		return now.AddDate(0, 0, -30).UnixMilli(), toMs, "day", "30d"
	case "90d":
		return now.AddDate(0, 0, -90).UnixMilli(), toMs, "day", "90d"
	default:
		return now.AddDate(0, 0, -7).UnixMilli(), toMs, "day", "7d"
	}
}

func rangeOptions(sel string) []rangeOpt {
	opts := []rangeOpt{
		{"today", "Today", false},
		{"24h", "24 hours", false},
		{"7d", "7 days", false},
		{"30d", "30 days", false},
		{"90d", "90 days", false},
	}
	for i := range opts {
		opts[i].Selected = opts[i].Key == sel
	}
	return opts
}

// ---- handlers ----

func (h *Handlers) currentSite(r *http.Request) *model.Site {
	all := h.reg.All()
	if len(all) == 0 {
		return nil
	}
	if v := r.URL.Query().Get("site"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			if s, ok := h.reg.ByID(id); ok {
				return s
			}
		}
	}
	return all[0]
}

func (h *Handlers) buildDashboard(site *model.Site, rangeKey string) (*viewData, error) {
	fromMs, toMs, interval, norm := resolveRange(rangeKey)

	summary, err := h.q.Summary(site.ID, fromMs, toMs)
	if err != nil {
		return nil, err
	}
	series, err := h.q.TimeSeries(site.ID, fromMs, toMs, interval)
	if err != nil {
		return nil, err
	}
	rt, err := h.q.Realtime(site.ID, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}

	vd := &viewData{
		Site:       site,
		Range:      norm,
		Ranges:     rangeOptions(norm),
		Summary:    summary,
		Chart:      areaChart(series, interval),
		Realtime:   rt,
		GeoEnabled: h.cfg.GeoEnabled,
	}

	identity := func(s string) string { return s }
	title := func(s string) string {
		if s == "" {
			return "—"
		}
		return capitalize(s)
	}
	countryDisp := func(s string) string {
		f := flag(s)
		if f != "" {
			return f + " " + countryName(s)
		}
		return countryName(s)
	}

	specs := []struct {
		dim, title, metric string
		disp               func(string) string
	}{
		{"path", "Top pages", "Views", identity},
		{"referrer", "Referrers", "Views", identity},
		{"country", "Countries", "Views", countryDisp},
		{"browser", "Browsers", "Views", identity},
		{"os", "Operating systems", "Views", identity},
		{"device", "Devices", "Views", title},
		{"entry", "Entry pages", "Visits", identity},
		{"exit", "Exit pages", "Visits", identity},
		{"language", "Languages", "Views", identity},
		{"event", "Custom events", "Count", identity},
	}
	for _, sp := range specs {
		rows, err := h.q.Top(site.ID, sp.dim, fromMs, toMs, 8)
		if err != nil {
			return nil, err
		}
		if sp.dim == "country" && !h.cfg.GeoEnabled {
			continue
		}
		vd.Panels = append(vd.Panels, buildPanel(sp.title, sp.metric, rows, sp.disp))
	}

	return vd, nil
}

func buildPanel(title, metric string, rows []query.Row, disp func(string) string) panel {
	p := panel{Title: title, Metric: metric}
	max := 1
	for _, r := range rows {
		if r.Count > max {
			max = r.Count
		}
	}
	for _, r := range rows {
		p.Rows = append(p.Rows, barRow{
			Display:  disp(r.Value),
			Count:    r.Count,
			Visitors: r.Visitors,
			Percent:  float64(r.Count) / float64(max) * 100,
		})
	}
	return p
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	site := h.currentSite(r)
	if site == nil {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	vd, err := h.buildDashboard(site, r.URL.Query().Get("range"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vd.Title = site.Name
	vd.Sites = h.reg.All()
	h.render(w, "index", vd)
}

// Realtime is an HTMX fragment polled by the dashboard.
func (h *Handlers) Realtime(w http.ResponseWriter, r *http.Request) {
	site := h.currentSite(r)
	if site == nil {
		_, _ = w.Write([]byte("0"))
		return
	}
	n, _ := h.q.Realtime(site.ID, time.Now().UnixMilli())
	_, _ = w.Write([]byte(strconv.Itoa(n)))
}

// Share renders a read-only public dashboard for a site's share token.
func (h *Handlers) Share(w http.ResponseWriter, r *http.Request) {
	tok := chi.URLParam(r, "token")
	site, ok := h.reg.ByShare(tok)
	if !ok || !site.Public {
		http.NotFound(w, r)
		return
	}
	vd, err := h.buildDashboard(site, r.URL.Query().Get("range"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vd.Title = site.Name
	vd.Public = true
	vd.ShareToken = tok
	h.render(w, "index", vd)
}
