package dashboard

import (
	"fmt"
	"strconv"
	"strings"
)

func fmtNum(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func fmtPct(f float64) string {
	return fmt.Sprintf("%.0f%%", f*100)
}

func fmtDuration(seconds float64) string {
	s := int(seconds + 0.5)
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s = s % 60
	if m < 60 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := m / 60
	m = m % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

// flag turns a two-letter ISO country code into its emoji flag.
func flag(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 {
		return ""
	}
	var r []rune
	for _, c := range code {
		if c < 'A' || c > 'Z' {
			return ""
		}
		r = append(r, 0x1F1E6+(c-'A'))
	}
	return string(r)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func countryName(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if n, ok := countries[code]; ok {
		return n
	}
	if code == "" {
		return "Unknown"
	}
	return code
}

var countries = map[string]string{
	"US": "United States", "GB": "United Kingdom", "DE": "Germany", "FR": "France",
	"NL": "Netherlands", "SE": "Sweden", "NO": "Norway", "DK": "Denmark",
	"FI": "Finland", "IS": "Iceland", "ES": "Spain", "IT": "Italy",
	"PT": "Portugal", "PL": "Poland", "CZ": "Czechia", "AT": "Austria",
	"CH": "Switzerland", "BE": "Belgium", "IE": "Ireland", "CA": "Canada",
	"AU": "Australia", "NZ": "New Zealand", "JP": "Japan", "CN": "China",
	"IN": "India", "BR": "Brazil", "MX": "Mexico", "RU": "Russia",
	"UA": "Ukraine", "TR": "Turkey", "ZA": "South Africa", "KR": "South Korea",
	"SG": "Singapore", "HK": "Hong Kong", "TW": "Taiwan", "ID": "Indonesia",
	"AR": "Argentina", "CL": "Chile", "CO": "Colombia", "GR": "Greece",
	"RO": "Romania", "HU": "Hungary", "BG": "Bulgaria", "HR": "Croatia",
	"SK": "Slovakia", "SI": "Slovenia", "EE": "Estonia", "LV": "Latvia",
	"LT": "Lithuania", "IL": "Israel", "AE": "UAE", "SA": "Saudi Arabia",
	"TH": "Thailand", "VN": "Vietnam", "PH": "Philippines", "MY": "Malaysia",
}
