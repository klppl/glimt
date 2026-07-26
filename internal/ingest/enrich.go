package ingest

import (
	"net/url"
	"strings"
	"time"

	"github.com/klppl/glimt/internal/model"
	"github.com/klppl/glimt/internal/referrer"
	"github.com/klppl/glimt/internal/ua"
)

// Payload is the compact JSON the tracking snippet posts. Field names are kept
// short to keep the snippet tiny; they carry no meaning to ad-block lists.
type Payload struct {
	N    string            `json:"n"`    // event name ("" or "pageview" => pageview)
	U    string            `json:"u"`    // current full URL
	R    string            `json:"r"`    // document.referrer
	W    int               `json:"w"`    // screen width
	H    int               `json:"h"`    // screen height
	L    string            `json:"l"`    // navigator.language
	T    string            `json:"t"`    // document.title
	HN   string            `json:"hn"`   // hostname
	LCP  float64           `json:"lcp"`  // Core Web Vitals
	INP  float64           `json:"inp"`  // Core Web Vitals
	CLS  float64           `json:"cls"`  // Core Web Vitals
	TTFB float64           `json:"ttfb"` // Core Web Vitals
	Val  float64           `json:"val"`  // event numeric value / revenue
	P    map[string]string `json:"p"`    // optional custom props
}

// build converts a raw payload + request context into a storable Event. The IP
// is consumed here (hash + geo) and intentionally not copied onto the Event.
// geoCountry, when non-empty (e.g. from Cloudflare's CF-IPCountry header), is
// used directly and the local GeoIP lookup is skipped.
func (in *Ingestor) build(site *model.Site, p Payload, ip, userAgent, acceptLang, geoCountry string) *model.Event {
	now := time.Now().UnixMilli()
	ev := &model.Event{
		WebsiteID:   site.ID,
		TS:          now,
		VisitorHash: in.salt.Hash(site.ID, ip, userAgent),
		PageTitle:   truncate(p.T, 255),
		Hostname:    truncate(p.HN, 100),
		LCP:         p.LCP,
		INP:         p.INP,
		CLS:         p.CLS,
		TTFB:        p.TTFB,
		Val:         p.Val,
	}

	r := ua.Parse(userAgent)
	ev.Browser, ev.BrowserVer, ev.OS, ev.Device = r.Browser, r.BrowserVer, r.OS, r.Device

	if geoCountry != "" {
		ev.Country = geoCountry // CF provides country only, no subdivision
	} else {
		ev.Country, ev.Region, ev.City = in.geo.Lookup(ip)
	}

	if u, err := url.Parse(p.U); err == nil {
		ev.URLPath = cleanPath(u.Path)
		if ev.Hostname == "" {
			ev.Hostname = truncate(u.Hostname(), 100)
		}
		q := u.Query()
		ev.UTMSource = q.Get("utm_source")
		ev.UTMMedium = q.Get("utm_medium")
		ev.UTMCampaign = q.Get("utm_campaign")
		ev.UTMTerm = q.Get("utm_term")
		ev.UTMContent = q.Get("utm_content")
	}

	ev.Referrer = p.R
	ev.RefClass, ev.RefSource = referrer.Classify(p.R, site.Domain)

	if p.N == "" || p.N == "pageview" {
		ev.Type = model.TypePageview
	} else {
		ev.Type = model.TypeCustom
		ev.Name = truncate(p.N, 80)
	}

	ev.ScreenBucket = screenBucket(p.W)

	lang := p.L
	if lang == "" {
		lang = firstLang(acceptLang)
	}
	ev.Language = normLang(lang)

	if len(p.P) > 0 {
		ev.Props = make(map[string]string, len(p.P))
		for k, v := range p.P {
			ev.Props[truncate(k, 60)] = truncate(v, 200)
		}
	}
	return ev
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimRight(p, "/")
	}
	return truncate(p, 512)
}

func screenBucket(w int) string {
	switch {
	case w <= 0:
		return ""
	case w < 640:
		return "small"
	case w < 1024:
		return "medium"
	case w < 1440:
		return "large"
	default:
		return "xlarge"
	}
}

func firstLang(accept string) string {
	if accept == "" {
		return ""
	}
	if i := strings.IndexByte(accept, ','); i >= 0 {
		accept = accept[:i]
	}
	if i := strings.IndexByte(accept, ';'); i >= 0 {
		accept = accept[:i]
	}
	return strings.TrimSpace(accept)
}

// normLang reduces a language tag to its primary subtag, e.g. "en-US" -> "en".
func normLang(l string) string {
	l = strings.ToLower(strings.TrimSpace(l))
	if i := strings.IndexByte(l, '-'); i >= 0 {
		l = l[:i]
	}
	if len(l) > 8 {
		return ""
	}
	return l
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
