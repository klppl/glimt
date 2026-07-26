package dashboard

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/klppl/glimt/internal/auth"
	"github.com/klppl/glimt/internal/model"
	"github.com/klppl/glimt/internal/query"
)

func (h *Handlers) LoginForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "login", &viewData{Title: "Sign in"})
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	tok, err := h.auth.Login(r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		h.render(w, "login", &viewData{Title: "Sign in", Error: "Invalid username or password."})
		return
	}
	h.auth.SetCookie(w, tok)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.CookieName); err == nil {
		h.auth.Logout(c.Value)
	}
	h.auth.ClearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type siteSetting struct {
	Site         *model.Site
	ScriptURL    string
	Snippet      string
	PixelSnippet string
	ShareURL     string
}

func (h *Handlers) Settings(w http.ResponseWriter, r *http.Request) {
	h.renderSettings(w, r, "")
}

func (h *Handlers) renderSettings(w http.ResponseWriter, r *http.Request, newAPIKey string) {
	// Prefer the configured base URL; otherwise derive it from the request host
	// so the copy-paste install snippet is always a working absolute URL.
	base := h.cfg.BaseURL
	if base == "" {
		base = "//" + r.Host
	}
	var rows []siteSetting
	for _, s := range h.reg.All() {
		ss := siteSetting{
			Site:         s,
			ScriptURL:    base + "/s/" + s.ScriptToken + ".js",
			Snippet:      `<script defer src="` + base + "/s/" + s.ScriptToken + `.js"></script>`,
			PixelSnippet: `<img src="` + base + "/pixel/" + s.CollectToken + `.gif" alt="" />`,
		}
		if s.Public && s.ShareToken != "" {
			ss.ShareURL = base + "/p/" + s.ShareToken
		}
		rows = append(rows, ss)
	}

	users, _ := h.auth.ListUsers()

	vd := &viewData{
		Title:      "Settings",
		Sites:      h.reg.All(),
		SiteRows:   rows,
		Users:      users,
		GeoEnabled: h.cfg.GeoEnabled,
		NewAPIKey:  newAPIKey,
		BaseURL:    base,
	}
	h.render(w, "settings", vd)
}

func (h *Handlers) SiteCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	_, apiKey, err := h.reg.Create(name, r.FormValue("domain"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.renderSettings(w, r, apiKey)
}

func (h *Handlers) SiteDelete(w http.ResponseWriter, r *http.Request) {
	id := formID(r)
	if id > 0 {
		_ = h.reg.Delete(id)
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handlers) SiteRegen(w http.ResponseWriter, r *http.Request) {
	id := formID(r)
	if id > 0 {
		_ = h.reg.RegenTokens(id)
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handlers) SitePublic(w http.ResponseWriter, r *http.Request) {
	id := formID(r)
	if id > 0 {
		_ = h.reg.SetPublic(id, r.FormValue("public") == "1")
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handlers) UserCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	user := r.FormValue("username")
	pass := r.FormValue("password")
	role := r.FormValue("role")
	if user != "" && pass != "" {
		_ = h.auth.CreateUser(user, pass, role)
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handlers) UserDelete(w http.ResponseWriter, r *http.Request) {
	id := formID(r)
	if id > 0 {
		_ = h.auth.DeleteUser(id)
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handlers) FunnelCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	siteID, _ := strconv.ParseInt(r.FormValue("site_id"), 10, 64)
	name := r.FormValue("name")
	step1 := r.FormValue("step1")
	step2 := r.FormValue("step2")
	step3 := r.FormValue("step3")

	if siteID > 0 && name != "" && step1 != "" && step2 != "" {
		steps := []query.FunnelStep{
			{Name: strings.TrimPrefix(step1, "event:"), IsURL: !strings.HasPrefix(step1, "event:")},
			{Name: strings.TrimPrefix(step2, "event:"), IsURL: !strings.HasPrefix(step2, "event:")},
		}
		if step3 != "" {
			steps = append(steps, query.FunnelStep{
				Name:  strings.TrimPrefix(step3, "event:"),
				IsURL: !strings.HasPrefix(step3, "event:"),
			})
		}
		_, _ = h.q.CreateFunnel(siteID, name, steps)
	}
	http.Redirect(w, r, "/?site="+strconv.FormatInt(siteID, 10), http.StatusSeeOther)
}

func (h *Handlers) FunnelDelete(w http.ResponseWriter, r *http.Request) {
	id := formID(r)
	siteID := r.FormValue("site_id")
	if id > 0 {
		_ = h.q.DeleteFunnel(id)
	}
	http.Redirect(w, r, "/?site="+siteID, http.StatusSeeOther)
}

func formID(r *http.Request) int64 {
	if err := r.ParseForm(); err != nil {
		return 0
	}
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	return id
}


