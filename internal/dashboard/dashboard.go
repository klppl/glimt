// Package dashboard renders the server-side, HTMX-driven admin UI and the
// public share view. Charts are inline SVG; top-N panels are HTML bars.
package dashboard

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
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
		"add1":    func(i int) int { return i + 1 },
	}
}

func (h *Handlers) parseTemplates() error {
	h.pages = map[string]*template.Template{}
	for _, page := range []string{"home", "index", "login", "settings"} {
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

// siteCard is one tile on the overview landing page.
type siteCard struct {
	Name     string
	Href     string
	Summary  query.Summary
	Realtime int
	Combined bool
}

type viewData struct {
	Title        string
	Sites        []*model.Site
	Site         *model.Site
	SiteRows     []siteSetting
	Users        []auth.User
	Funnels      []query.Funnel
	FunnelData   []query.FunnelStepResult
	ActiveFunnel *query.Funnel
	Range        string
	FromDate     string
	ToDate       string
	Ranges       []rangeOpt
	Summary      query.Summary
	Vitals       query.Vitals
	Chart        template.HTML
	Panels       []panel
	Realtime     int
	GeoEnabled   bool
	Public       bool
	ShareToken   string
	NewAPIKey    string
	BaseURL      string
	Error        string
	Cards        []siteCard // overview landing page tiles
}

// ---- range handling ----

func resolveRange(key string) (fromMs, toMs int64, interval, norm string) {
	fromMs, toMs, interval, norm, _, _ = resolveRangeParams(key, "", "")
	return
}

func resolveRangeParams(key, fromStr, toStr string) (fromMs, toMs int64, interval, norm, fromDate, toDate string) {
	now := time.Now().UTC()
	toMs = now.UnixMilli()
	switch key {
	case "today":
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return from.UnixMilli(), toMs, "hour", "today", "", ""
	case "24h":
		return now.Add(-24 * time.Hour).UnixMilli(), toMs, "hour", "24h", "", ""
	case "30d":
		return now.AddDate(0, 0, -30).UnixMilli(), toMs, "day", "30d", "", ""
	case "90d":
		return now.AddDate(0, 0, -90).UnixMilli(), toMs, "day", "90d", "", ""
	case "custom":
		if fromStr == "" || toStr == "" {
			dFrom := now.AddDate(0, 0, -7)
			return dFrom.UnixMilli(), toMs, "day", "custom", dFrom.Format("2006-01-02"), now.Format("2006-01-02")
		}
		tFrom, errFrom := time.Parse("2006-01-02", fromStr)
		tTo, errTo := time.Parse("2006-01-02", toStr)
		if errFrom != nil || errTo != nil {
			dFrom := now.AddDate(0, 0, -7)
			return dFrom.UnixMilli(), toMs, "day", "custom", dFrom.Format("2006-01-02"), now.Format("2006-01-02")
		}
		fMs := tFrom.UTC().UnixMilli()
		tToNext := time.Date(tTo.Year(), tTo.Month(), tTo.Day(), 23, 59, 59, 999000000, time.UTC)
		tMs := tToNext.UnixMilli()
		if tMs < fMs {
			dFrom := now.AddDate(0, 0, -7)
			return dFrom.UnixMilli(), toMs, "day", "custom", dFrom.Format("2006-01-02"), now.Format("2006-01-02")
		}
		inter := "day"
		if tMs-fMs <= int64(48*time.Hour/time.Millisecond) {
			inter = "hour"
		}
		return fMs, tMs, inter, "custom", tFrom.Format("2006-01-02"), tTo.Format("2006-01-02")
	default:
		return now.AddDate(0, 0, -7).UnixMilli(), toMs, "day", "7d", "", ""
	}
}

func rangeOptions(sel string) []rangeOpt {
	opts := []rangeOpt{
		{"today", "Today", false},
		{"24h", "24 hours", false},
		{"7d", "7 days", false},
		{"30d", "30 days", false},
		{"90d", "90 days", false},
		{"custom", "Custom", false},
	}
	for i := range opts {
		opts[i].Selected = opts[i].Key == sel
	}
	return opts
}

// ---- handlers ----

// allSitesID is the sentinel site ID for the combined "All websites" view; the
// query layer treats 0 as "every site". Real sites always have positive rowids.
const allSitesID int64 = 0

// aggregateSite is the pseudo-site selected when viewing all websites combined.
func aggregateSite() *model.Site { return &model.Site{ID: allSitesID, Name: "All websites"} }

func (h *Handlers) currentSite(r *http.Request) *model.Site {
	all := h.reg.All()
	if len(all) == 0 {
		return nil
	}
	if v := r.URL.Query().Get("site"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			if id == allSitesID {
				return aggregateSite()
			}
			if s, ok := h.reg.ByID(id); ok {
				return s
			}
		}
	}
	return all[0]
}

func (h *Handlers) buildDashboard(site *model.Site, rangeKey, fromStr, toStr string) (*viewData, error) {
	fromMs, toMs, interval, norm, fromDate, toDate := resolveRangeParams(rangeKey, fromStr, toStr)

	summary, err := h.q.Summary(site.ID, fromMs, toMs)
	if err != nil {
		return nil, err
	}
	vitals, err := h.q.Vitals(site.ID, fromMs, toMs)
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

	funnels, _ := h.q.ListFunnels(site.ID)
	var activeFunnel *query.Funnel
	var funnelResults []query.FunnelStepResult
	if len(funnels) > 0 {
		activeFunnel = &funnels[0]
		funnelResults, _ = h.q.EvaluateFunnel(site.ID, activeFunnel.Steps, fromMs, toMs)
	}

	vd := &viewData{
		Site:         site,
		Range:        norm,
		FromDate:     fromDate,
		ToDate:       toDate,
		Ranges:       rangeOptions(norm),
		Summary:      summary,
		Vitals:       vitals,
		Funnels:      funnels,
		ActiveFunnel: activeFunnel,
		FunnelData:   funnelResults,
		Chart:        areaChart(series, interval),
		Realtime:     rt,
		GeoEnabled:   h.cfg.GeoEnabled,
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
		{"title", "Page titles", "Views", identity},
		{"hostname", "Hostnames", "Views", identity},
		{"referrer", "Referrers", "Views", identity},
		{"country", "Countries", "Views", countryDisp},
		{"city", "Cities", "Views", identity},
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

// Index serves the overview landing page (a tile per site plus an "All
// combined" tile) at "/", and a single site's detailed dashboard when a "site"
// query param is present (a tile was clicked).
func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	if !r.URL.Query().Has("site") {
		h.home(w, r)
		return
	}
	site := h.currentSite(r)
	if site == nil {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	vd, err := h.buildDashboard(site, r.URL.Query().Get("range"), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vd.Title = site.Name
	vd.Sites = h.reg.All()
	h.render(w, "index", vd)
}

// home renders the overview grid: one summary tile per site, plus a leading
// "All combined" tile (site 0) when more than one site exists.
func (h *Handlers) home(w http.ResponseWriter, r *http.Request) {
	all := h.reg.All()
	if len(all) == 0 {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	fromMs, toMs, _, norm, fromDate, toDate := resolveRangeParams(r.URL.Query().Get("range"), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	now := time.Now().UnixMilli()

	card := func(id int64, name string, combined bool) (siteCard, error) {
		sum, err := h.q.Summary(id, fromMs, toMs)
		if err != nil {
			return siteCard{}, err
		}
		rt, err := h.q.Realtime(id, now)
		if err != nil {
			return siteCard{}, err
		}
		href := fmt.Sprintf("/?site=%d&range=%s", id, norm)
		if norm == "custom" && fromDate != "" && toDate != "" {
			href += fmt.Sprintf("&from=%s&to=%s", fromDate, toDate)
		}
		return siteCard{
			Name:     name,
			Href:     href,
			Summary:  sum,
			Realtime: rt,
			Combined: combined,
		}, nil
	}

	vd := &viewData{
		Title:      "Overview",
		Range:      norm,
		FromDate:   fromDate,
		ToDate:     toDate,
		Ranges:     rangeOptions(norm),
		GeoEnabled: h.cfg.GeoEnabled,
	}
	if len(all) > 1 {
		c, err := card(allSitesID, "All combined", true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		vd.Cards = append(vd.Cards, c)
	}
	for _, s := range all {
		c, err := card(s.ID, s.Name, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		vd.Cards = append(vd.Cards, c)
	}
	h.render(w, "home", vd)
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
	vd, err := h.buildDashboard(site, r.URL.Query().Get("range"), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vd.Title = site.Name
	vd.Public = true
	vd.ShareToken = tok
	h.render(w, "index", vd)
}

// Export streams raw event logs for a site as CSV or JSON.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	site := h.currentSite(r)
	if site == nil {
		http.Error(w, "site not found", http.StatusNotFound)
		return
	}
	fromMs, toMs, _, norm, _, _ := resolveRangeParams(r.URL.Query().Get("range"), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	events, err := h.q.ExportEvents(site.ID, fromMs, toMs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rangeKey := norm
	if rangeKey == "" {
		rangeKey = "7d"
	}
	domain := site.Domain
	if domain == "" {
		domain = "site"
	}

	format := r.URL.Query().Get("format")
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"glimt-%s-%s.json\"", domain, rangeKey))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(events)
		return
	}

	// Default CSV
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"glimt-%s-%s.csv\"", domain, rangeKey))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Timestamp", "Type", "Name", "URL Path", "Page Title", "Hostname", "Referrer", "Browser", "OS", "Device", "Country", "Language", "LCP", "INP", "CLS", "TTFB", "Value"})
	for _, e := range events {
		tStr := time.UnixMilli(e.TS).UTC().Format(time.RFC3339)
		_ = cw.Write([]string{
			tStr, e.Type, e.Name, e.URLPath, e.PageTitle, e.Hostname, e.Referrer,
			e.Browser, e.OS, e.Device, e.Country, e.Language,
			fmt.Sprintf("%.0f", e.LCP), fmt.Sprintf("%.0f", e.INP), fmt.Sprintf("%.3f", e.CLS), fmt.Sprintf("%.0f", e.TTFB),
			fmt.Sprintf("%.2f", e.Val),
		})
	}
	cw.Flush()
}
