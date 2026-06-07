// Package query holds the dashboard read models. Timeseries and top-N panels
// come from the rollup tables; accurate range-unique visitor counts and
// session-level KPIs come from the session table directly.
package query

import (
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

// Summary computes headline KPIs for a site over [fromMs, toMs).
func (q *Querier) Summary(siteID, fromMs, toMs int64) (Summary, error) {
	var s Summary

	fromHour := fromMs / msPerHour
	toHour := toMs / msPerHour // inclusive: the current (partial) hour bucket counts
	if err := q.db.R.QueryRow(
		`SELECT COALESCE(SUM(pageviews),0) FROM rollup_stats
		 WHERE website_id = ? AND bucket_hour >= ? AND bucket_hour <= ?`,
		siteID, fromHour, toHour).Scan(&s.Pageviews); err != nil {
		return s, err
	}

	if err := q.db.R.QueryRow(
		`SELECT COUNT(DISTINCT visitor_hash), COUNT(*),
		        COALESCE(AVG(is_bounce),0), COALESCE(AVG(last_seen_at - started_at),0)
		 FROM session
		 WHERE website_id = ? AND started_at >= ? AND started_at < ?`,
		siteID, fromMs, toMs).Scan(&s.Visitors, &s.Visits, &s.BounceRate, &s.AvgDuration); err != nil {
		return s, err
	}
	s.AvgDuration /= 1000 // ms -> seconds
	return s, nil
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
			 WHERE website_id = ? AND bucket_hour >= ? AND bucket_hour <= ?
			 GROUP BY bucket_hour/24`,
			msPerDay, siteID, fromMs/msPerHour, toMs/msPerHour)
		if err != nil {
			return nil, err
		}
		rows = r
	} else {
		r, err := q.db.R.Query(
			`SELECT bucket_hour*?, pageviews, visitors
			 FROM rollup_stats
			 WHERE website_id = ? AND bucket_hour >= ? AND bucket_hour <= ?`,
			msPerHour, siteID, fromMs/msPerHour, toMs/msPerHour)
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
	rows, err := q.db.R.Query(
		`SELECT value, SUM(pageviews) AS c, SUM(visitors) AS v
		 FROM rollup_dim
		 WHERE website_id = ? AND dimension = ? AND bucket_day >= ? AND bucket_day <= ?
		 GROUP BY value ORDER BY c DESC LIMIT ?`,
		siteID, dimension, fromDay, toDay, limit)
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
	err := q.db.R.QueryRow(
		`SELECT COUNT(DISTINCT visitor_hash) FROM event
		 WHERE website_id = ? AND ts > ?`,
		siteID, nowMs-5*60*1000).Scan(&n)
	return n, err
}
