package dashboard

import (
	"net/http"
	"strconv"

	"github.com/klppl/glimt/internal/auth"
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
			Site:      s,
			ScriptURL: base + "/s/" + s.ScriptToken + ".js",
			Snippet:   `<script defer src="` + base + "/s/" + s.ScriptToken + `.js"></script>`,
		}
		if s.Public && s.ShareToken != "" {
			ss.ShareURL = base + "/p/" + s.ShareToken
		}
		rows = append(rows, ss)
	}
	vd := &viewData{
		Title:      "Settings",
		Sites:      h.reg.All(),
		SiteRows:   rows,
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

func formID(r *http.Request) int64 {
	if err := r.ParseForm(); err != nil {
		return 0
	}
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	return id
}
