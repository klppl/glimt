package dashboard

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/klppl/glimt/internal/query"
)

// areaChart renders a minimal inline-SVG area+line chart from a series. No JS,
// no external chart library — just scaled paths on a responsive viewBox.
func areaChart(series []query.Point, interval string) template.HTML {
	const (
		w   = 720.0
		h   = 220.0
		pad = 24.0
	)
	if len(series) == 0 {
		return template.HTML(`<div class="chart-empty">No data in this range yet.</div>`)
	}

	maxV := 1
	for _, p := range series {
		if p.Pageviews > maxV {
			maxV = p.Pageviews
		}
	}

	innerW := w - 2*pad
	innerH := h - 2*pad
	n := len(series)

	x := func(i int) float64 {
		if n == 1 {
			return pad + innerW/2
		}
		return pad + float64(i)*innerW/float64(n-1)
	}
	y := func(v int) float64 {
		return pad + innerH - float64(v)/float64(maxV)*innerH
	}

	var line strings.Builder
	var area strings.Builder
	fmt.Fprintf(&area, "M %.1f %.1f ", x(0), h-pad)
	for i, p := range series {
		cmd := "L"
		if i == 0 {
			cmd = "M"
		}
		fmt.Fprintf(&line, "%s %.1f %.1f ", cmd, x(i), y(p.Pageviews))
		fmt.Fprintf(&area, "L %.1f %.1f ", x(i), y(p.Pageviews))
	}
	fmt.Fprintf(&area, "L %.1f %.1f Z", x(n-1), h-pad)

	// A few x-axis labels (start, middle, end).
	labels := axisLabels(series, interval)
	var ticks strings.Builder
	for _, l := range labels {
		fmt.Fprintf(&ticks,
			`<text x="%.1f" y="%.1f" class="ax">%s</text>`,
			x(l.idx), h-6, template.HTMLEscapeString(l.text))
	}

	svg := fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" class="chart" preserveAspectRatio="none" role="img">
<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="axis"/>
<path d="%s" class="area"/>
<path d="%s" class="line"/>
%s
</svg>`,
		w, h,
		pad, h-pad, w-pad, h-pad,
		area.String(), line.String(), ticks.String())

	return template.HTML(svg)
}

type axisLabel struct {
	idx  int
	text string
}

func axisLabels(series []query.Point, interval string) []axisLabel {
	n := len(series)
	if n == 0 {
		return nil
	}
	idxs := []int{0}
	if n > 2 {
		idxs = append(idxs, n/2)
	}
	if n > 1 {
		idxs = append(idxs, n-1)
	}
	layout := "Jan 2"
	if interval == "hour" {
		layout = "15:04"
	}
	var out []axisLabel
	for _, i := range idxs {
		t := time.UnixMilli(series[i].Bucket).UTC()
		out = append(out, axisLabel{idx: i, text: t.Format(layout)})
	}
	return out
}
