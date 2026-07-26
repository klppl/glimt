// Package query holds the dashboard read models. Timeseries and top-N panels
// come from the rollup tables; accurate range-unique visitor counts and
// session-level KPIs come from the session table directly.
package query

import (
	"encoding/json"
	"time"

	"github.com/klppl/glimt/internal/store"
)

const (
	msPerHour = 3600 * 1000
	msPerDay  = 86400 * 1000
)

type Querier struct {
	db *store.DB
}

func New(db *store.DB) *Querier { return &Querier{db: db} }

type Summary struct {
	Pageviews   int
	Visitors    int     // distinct visitor hashes in range
	Visits      int     // sessions started in range
	BounceRate  float64 // 0..1
	AvgDuration float64 // seconds
	Revenue     float64 // sum of numerical event values / revenue in range
}

type Vitals struct {
	LCP   float64 // ms
	INP   float64 // ms
	CLS   float64 // unitless score
	TTFB  float64 // ms
	Count int
}

type ExportEvent struct {
	TS        int64   `json:"timestamp"`
	Type      string  `json:"type"`
	Name      string  `json:"name,omitempty"`
	URLPath   string  `json:"url_path"`
	PageTitle string  `json:"page_title,omitempty"`
	Hostname  string  `json:"hostname,omitempty"`
	Referrer  string  `json:"referrer,omitempty"`
	Browser   string  `json:"browser,omitempty"`
	OS        string  `json:"os,omitempty"`
	Device    string  `json:"device,omitempty"`
	Country   string  `json:"country,omitempty"`
	Language  string  `json:"language,omitempty"`
	LCP       float64 `json:"lcp,omitempty"`
	INP       float64 `json:"inp,omitempty"`
	CLS       float64 `json:"cls,omitempty"`
	TTFB      float64 `json:"ttfb,omitempty"`
	Val       float64 `json:"val,omitempty"`
}

type Point struct {
	Bucket    int64 // unix ms at bucket start
	Pageviews int
	Visitors  int
}

type Row struct {
	Value    string
	Count    int
	Visitors int
}

// scope builds the website_id predicate for a query. A siteID of 0 means the
// combined "all websites" view and matches every row; any positive ID keeps the
// literal `website_id = ?` form so SQLite still uses the website_id index on the
// common single-site path. Real sites always have positive rowids.
func scope(siteID int64) (clause string, args []any) {
	if siteID == 0 {
		return "1=1", nil
	}
	return "website_id = ?", []any{siteID}
}

// Summary computes headline KPIs for a site over [fromMs, toMs).
func (q *Querier) Summary(siteID, fromMs, toMs int64) (Summary, error) {
	var s Summary
	where, args := scope(siteID)

	fromHour := fromMs / msPerHour
	toHour := toMs / msPerHour // inclusive: the current (partial) hour bucket counts
	if err := q.db.R.QueryRow(
		`SELECT COALESCE(SUM(pageviews),0) FROM rollup_stats
		 WHERE `+where+` AND bucket_hour >= ? AND bucket_hour <= ?`,
		append(args, fromHour, toHour)...).Scan(&s.Pageviews); err != nil {
		return s, err
	}

	if err := q.db.R.QueryRow(
		`SELECT COUNT(DISTINCT visitor_hash), COUNT(*),
		        COALESCE(AVG(is_bounce),0), COALESCE(AVG(last_seen_at - started_at),0)
		 FROM session
		 WHERE `+where+` AND started_at >= ? AND started_at < ?`,
		append(args, fromMs, toMs)...).Scan(&s.Visitors, &s.Visits, &s.BounceRate, &s.AvgDuration); err != nil {
		return s, err
	}
	s.AvgDuration /= 1000 // ms -> seconds

	// Compute revenue / numeric event values sum
	_ = q.db.R.QueryRow(
		`SELECT COALESCE(SUM(val),0) FROM event
		 WHERE `+where+` AND ts >= ? AND ts < ? AND val IS NOT NULL AND val > 0`,
		append(args, fromMs, toMs)...).Scan(&s.Revenue)

	return s, nil
}

// Vitals computes average Core Web Vitals over [fromMs, toMs).
func (q *Querier) Vitals(siteID, fromMs, toMs int64) (Vitals, error) {
	var v Vitals
	where, args := scope(siteID)
	err := q.db.R.QueryRow(
		`SELECT COALESCE(AVG(NULLIF(lcp, 0)), 0),
		        COALESCE(AVG(NULLIF(inp, 0)), 0),
		        COALESCE(AVG(NULLIF(cls, 0)), 0),
		        COALESCE(AVG(NULLIF(ttfb, 0)), 0),
		        COUNT(CASE WHEN lcp>0 OR inp>0 OR cls>0 OR ttfb>0 THEN 1 END)
		 FROM event
		 WHERE `+where+` AND ts >= ? AND ts < ?`,
		append(args, fromMs, toMs)...).Scan(&v.LCP, &v.INP, &v.CLS, &v.TTFB, &v.Count)
	return v, err
}

// ExportEvents retrieves event logs for raw CSV/JSON data export.
func (q *Querier) ExportEvents(siteID, fromMs, toMs int64) ([]ExportEvent, error) {
	where, args := scope(siteID)
	rows, err := q.db.R.Query(
		`SELECT ts, type, COALESCE(name,''), COALESCE(url_path,''), COALESCE(page_title,''), COALESCE(hostname,''),
		        COALESCE(referrer,''), COALESCE(browser,''), COALESCE(os,''), COALESCE(device,''),
		        COALESCE(country,''), COALESCE(language,''),
		        COALESCE(lcp,0), COALESCE(inp,0), COALESCE(cls,0), COALESCE(ttfb,0), COALESCE(val,0)
		 FROM event
		 WHERE `+where+` AND ts >= ? AND ts < ?
		 ORDER BY ts DESC LIMIT 10000`,
		append(args, fromMs, toMs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ExportEvent
	for rows.Next() {
		var e ExportEvent
		var typ int
		if err := rows.Scan(&e.TS, &typ, &e.Name, &e.URLPath, &e.PageTitle, &e.Hostname,
			&e.Referrer, &e.Browser, &e.OS, &e.Device,
			&e.Country, &e.Language,
			&e.LCP, &e.INP, &e.CLS, &e.TTFB, &e.Val); err != nil {
			return nil, err
		}
		if typ == 0 {
			e.Type = "pageview"
		} else {
			e.Type = "custom"
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// TimeSeries returns zero-filled buckets. interval is "hour" or "day".
func (q *Querier) TimeSeries(siteID, fromMs, toMs int64, interval string) ([]Point, error) {
	step := int64(msPerHour)
	if interval == "day" {
		step = msPerDay
	}

	// Align the start down to the bucket boundary.
	start := (fromMs / step) * step
	counts := map[int64]*Point{}
	where, args := scope(siteID)

	var rows interface {
		Next() bool
		Scan(...any) error
		Close() error
		Err() error
	}

	if interval == "day" {
		r, err := q.db.R.Query(
			`SELECT (bucket_hour/24)*?, SUM(pageviews), SUM(visitors)
			 FROM rollup_stats
			 WHERE `+where+` AND bucket_hour >= ? AND bucket_hour <= ?
			 GROUP BY bucket_hour/24`,
			append([]any{msPerDay}, append(args, fromMs/msPerHour, toMs/msPerHour)...)...)
		if err != nil {
			return nil, err
		}
		rows = r
	} else {
		r, err := q.db.R.Query(
			`SELECT bucket_hour*?, SUM(pageviews), SUM(visitors)
			 FROM rollup_stats
			 WHERE `+where+` AND bucket_hour >= ? AND bucket_hour <= ?
			 GROUP BY bucket_hour`,
			append([]any{msPerHour}, append(args, fromMs/msPerHour, toMs/msPerHour)...)...)
		if err != nil {
			return nil, err
		}
		rows = r
	}
	defer rows.Close()

	for rows.Next() {
		var b int64
		var pv, vis int
		if err := rows.Scan(&b, &pv, &vis); err != nil {
			return nil, err
		}
		counts[b] = &Point{Bucket: b, Pageviews: pv, Visitors: vis}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []Point
	for t := start; t < toMs; t += step {
		if p, ok := counts[t]; ok {
			out = append(out, *p)
		} else {
			out = append(out, Point{Bucket: t})
		}
	}
	return out, nil
}

// Top returns the top values for a dimension over the range, ordered by count.
func (q *Querier) Top(siteID int64, dimension string, fromMs, toMs int64, limit int) ([]Row, error) {
	fromDay := fromMs / msPerDay
	toDay := toMs / msPerDay
	where, args := scope(siteID)
	rows, err := q.db.R.Query(
		`SELECT value, SUM(pageviews) AS c, SUM(visitors) AS v
		 FROM rollup_dim
		 WHERE `+where+` AND dimension = ? AND bucket_day >= ? AND bucket_day <= ?
		 GROUP BY value ORDER BY c DESC LIMIT ?`,
		append(args, dimension, fromDay, toDay, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.Value, &r.Count, &r.Visitors); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Realtime counts distinct visitors active in the last 5 minutes.
func (q *Querier) Realtime(siteID, nowMs int64) (int, error) {
	var n int
	where, args := scope(siteID)
	err := q.db.R.QueryRow(
		`SELECT COUNT(DISTINCT visitor_hash) FROM event
		 WHERE `+where+` AND ts > ?`,
		append(args, nowMs-5*60*1000)...).Scan(&n)
	return n, err
}

type FunnelStep struct {
	Name  string `json:"name"`  // e.g. "/checkout" or "purchase"
	IsURL bool   `json:"is_url"` // true if URL path, false if event name
}

type FunnelStepResult struct {
	Step       string  `json:"step"`
	Count      int     `json:"count"`
	Percentage float64 `json:"percentage"`
	Dropoff    float64 `json:"dropoff"`
}

type Funnel struct {
	ID        int64        `json:"id"`
	WebsiteID int64        `json:"website_id"`
	Name      string       `json:"name"`
	Steps     []FunnelStep `json:"steps"`
	CreatedAt int64        `json:"created_at"`
}

func (q *Querier) CreateFunnel(siteID int64, name string, steps []FunnelStep) (*Funnel, error) {
	b, err := json.Marshal(steps)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	res, err := q.db.W.Exec(
		`INSERT INTO funnel(website_id, name, steps_json, created_at) VALUES(?,?,?,?)`,
		siteID, name, string(b), now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Funnel{ID: id, WebsiteID: siteID, Name: name, Steps: steps, CreatedAt: now}, nil
}

func (q *Querier) ListFunnels(siteID int64) ([]Funnel, error) {
	where, args := scope(siteID)
	rows, err := q.db.R.Query(
		`SELECT id, website_id, name, steps_json, created_at FROM funnel WHERE `+where+` ORDER BY created_at DESC`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Funnel
	for rows.Next() {
		var fn Funnel
		var sJSON string
		if err := rows.Scan(&fn.ID, &fn.WebsiteID, &fn.Name, &sJSON, &fn.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(sJSON), &fn.Steps)
		out = append(out, fn)
	}
	return out, rows.Err()
}

func (q *Querier) DeleteFunnel(id int64) error {
	_, err := q.db.W.Exec(`DELETE FROM funnel WHERE id = ?`, id)
	return err
}

func (q *Querier) EvaluateFunnel(siteID int64, steps []FunnelStep, fromMs, toMs int64) ([]FunnelStepResult, error) {
	if len(steps) == 0 {
		return nil, nil
	}

	where, args := scope(siteID)
	rows, err := q.db.R.Query(
		`SELECT session_id, ts, type, COALESCE(name,''), COALESCE(url_path,'')
		 FROM event
		 WHERE `+where+` AND ts >= ? AND ts < ?
		 ORDER BY session_id, ts ASC`,
		append(args, fromMs, toMs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type eventEntry struct {
		ts    int64
		isURL bool
		val   string
	}
	sessionEvents := map[int64][]eventEntry{}
	for rows.Next() {
		var sid, ts int64
		var typ int
		var name, path string
		if err := rows.Scan(&sid, &ts, &typ, &name, &path); err != nil {
			return nil, err
		}
		if typ == 0 {
			sessionEvents[sid] = append(sessionEvents[sid], eventEntry{ts: ts, isURL: true, val: path})
		} else {
			sessionEvents[sid] = append(sessionEvents[sid], eventEntry{ts: ts, isURL: false, val: name})
		}
	}

	stepCounts := make([]int, len(steps))
	for _, events := range sessionEvents {
		stepIdx := 0
		var lastTs int64 = 0
		for _, e := range events {
			if stepIdx >= len(steps) {
				break
			}
			targetStep := steps[stepIdx]
			if e.isURL == targetStep.IsURL && e.val == targetStep.Name && e.ts >= lastTs {
				stepCounts[stepIdx]++
				lastTs = e.ts
				stepIdx++
			}
		}
	}

	var results []FunnelStepResult
	baseCount := 0
	if len(stepCounts) > 0 {
		baseCount = stepCounts[0]
	}

	for i, st := range steps {
		cnt := stepCounts[i]
		pct := 0.0
		if baseCount > 0 {
			pct = float64(cnt) / float64(baseCount) * 100.0
		}
		dropoff := 0.0
		if i > 0 && stepCounts[i-1] > 0 {
			dropoff = float64(stepCounts[i-1]-cnt) / float64(stepCounts[i-1]) * 100.0
		}
		results = append(results, FunnelStepResult{
			Step:       st.Name,
			Count:      cnt,
			Percentage: pct,
			Dropoff:    dropoff,
		})
	}
	return results, nil
}
