// Package referrer classifies a referrer URL into a channel and a source name.
package referrer

import (
	"net/url"
	"strings"

	"github.com/klppl/glimt/internal/model"
)

// substring match on host -> source name
var searchEngines = map[string]string{
	"google.":      "Google",
	"bing.":        "Bing",
	"duckduckgo.":  "DuckDuckGo",
	"yahoo.":       "Yahoo",
	"yandex.":      "Yandex",
	"baidu.":       "Baidu",
	"ecosia.":      "Ecosia",
	"search.brave": "Brave",
	"startpage.":   "Startpage",
	"qwant.":       "Qwant",
}

var socialNetworks = map[string]string{
	"t.co":             "Twitter",
	"twitter.":         "Twitter",
	"x.com":            "X",
	"facebook.":        "Facebook",
	"fb.":              "Facebook",
	"instagram.":       "Instagram",
	"linkedin.":        "LinkedIn",
	"lnkd.in":          "LinkedIn",
	"reddit.":          "Reddit",
	"youtube.":         "YouTube",
	"youtu.be":         "YouTube",
	"pinterest.":       "Pinterest",
	"mastodon":         "Mastodon",
	"t.me":             "Telegram",
	"news.ycombinator": "Hacker News",
	"bsky.":            "Bluesky",
	"tiktok.":          "TikTok",
}

// Classify returns (class, source). ownDomain (may be empty) marks internal
// referrers so a site's own pages don't pollute the referrer report.
func Classify(ref, ownDomain string) (int, string) {
	if ref == "" {
		return model.RefDirect, ""
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return model.RefDirect, ""
	}
	host := strings.ToLower(u.Host)
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}

	if ownDomain != "" {
		od := strings.ToLower(ownDomain)
		if host == od || strings.HasSuffix(host, "."+od) {
			return model.RefInternal, ""
		}
	}
	for k, v := range searchEngines {
		if strings.Contains(host, k) {
			return model.RefSearch, v
		}
	}
	for k, v := range socialNetworks {
		if strings.Contains(host, k) {
			return model.RefSocial, v
		}
	}
	return model.RefReferral, strings.TrimPrefix(host, "www.")
}
