// Package ua turns a User-Agent string into coarse, non-identifying buckets.
package ua

import "github.com/mileusna/useragent"

type Result struct {
	Browser    string
	BrowserVer string
	OS         string
	Device     string // mobile | tablet | desktop | bot
}

func Parse(s string) Result {
	u := useragent.Parse(s)
	device := "desktop"
	switch {
	case u.Bot:
		device = "bot"
	case u.Tablet:
		device = "tablet"
	case u.Mobile:
		device = "mobile"
	}
	return Result{
		Browser:    u.Name,
		BrowserVer: majorVersion(u.Version),
		OS:         u.OS,
		Device:     device,
	}
}

// majorVersion keeps only the leading major component, e.g. "120.0.0" -> "120".
func majorVersion(v string) string {
	for i := 0; i < len(v); i++ {
		if v[i] == '.' {
			return v[:i]
		}
	}
	return v
}
