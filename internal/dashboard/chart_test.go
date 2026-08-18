package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/klppl/glimt/internal/query"
)

func TestAreaChartEmpty(t *testing.T) {
	html := areaChart(nil, "day")
	if !strings.Contains(string(html), "chart-empty") {
		t.Errorf("expected chart-empty for nil series, got: %s", html)
	}
}

func TestAreaChartDotsAndTooltips(t *testing.T) {
	// 2026-08-01 00:00:00 UTC
	t1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	t2 := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC).UnixMilli()

	series := []query.Point{
		{Bucket: t1, Pageviews: 42, Visitors: 12},
		{Bucket: t2, Pageviews: 1, Visitors: 1},
	}

	html := string(areaChart(series, "day"))

	// Check svg element and dots
	if !strings.Contains(html, `<svg viewBox="0 0 720 220" class="chart"`) {
		t.Errorf("expected viewBox and chart class in SVG, got: %s", html)
	}

	// Verify dots presence
	dot1 := `<circle class="dot" cx="24.0" cy="24.0" r="3.5"><title>Aug 1: 42 pageviews (12 visitors)</title></circle>`
	dot2 := `<circle class="dot" cx="696.0" cy="191.9" r="3.5"><title>Aug 2: 1 pageview (1 visitor)</title></circle>`

	if !strings.Contains(html, dot1) {
		t.Errorf("expected dot1 %q in SVG, got:\n%s", dot1, html)
	}
	if !strings.Contains(html, dot2) {
		t.Errorf("expected dot2 %q in SVG, got:\n%s", dot2, html)
	}
}

func TestAreaChartHourlyInterval(t *testing.T) {
	t1 := time.Date(2026, 8, 1, 14, 30, 0, 0, time.UTC).UnixMilli()
	series := []query.Point{
		{Bucket: t1, Pageviews: 5, Visitors: 3},
	}

	html := string(areaChart(series, "hour"))

	expectedTitle := "<title>14:30: 5 pageviews (3 visitors)</title>"
	if !strings.Contains(html, expectedTitle) {
		t.Errorf("expected title %q in SVG, got:\n%s", expectedTitle, html)
	}
}
