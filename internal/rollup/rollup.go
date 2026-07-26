// Package rollup pre-aggregates raw events into fast dashboard tables. Rather
// than fragile incremental deltas, it recomputes a trailing window each tick
// (recent hours for the timeseries, today+yesterday for dimension top-Ns). Once
// a bucket falls out of the window it is final and never touched again.
package rollup

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/klppl/glimt/internal/store"
)

const (
	interval        = 60 * time.Second
	statsHourWindow = 3 // recompute the last N hourly buckets
	dimDayWindow    = 2 // recompute the last N daily buckets (today + yesterday)

	msPerHour = 3600 * 1000
	msPerDay  = 86400 * 1000
)

// event-sourced dimensions: (dimension, column, pageview-only?)
var eventDims = []struct {
	name          string
	column        string
	pageviewsOnly bool
}{
	{"path", "url_path", true},
	{"title", "page_title", true},
	{"hostname", "hostname", false},
	{"country", "country", false},
	{"region", "region", false},
	{"city", "city", false},
	{"browser", "browser", false},
	{"os", "os", false},
	{"device", "device", false},
	{"referrer", "ref_source", false},
	{"language", "language", false},
}

type Worker struct {
	db *store.DB
}

func New(db *store.DB) *Worker { return &Worker{db: db} }

// Run blocks until ctx is cancelled, ticking every interval. It runs one pass
// immediately so the dashboard has data on first load.
func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(interval)
	defer t.Stop()
	w.tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.tick()
		}
	}
}

func (w *Worker) tick() {
	if err := w.recompute(time.Now().UTC()); err != nil {
		log.Printf("rollup: %v", err)
	}
}

func (w *Worker) recompute(now time.Time) error {
	tx, err := w.db.W.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	nowMs := now.UnixMilli()
	fromHour := nowMs/msPerHour - (statsHourWindow - 1)
	fromHourMs := fromHour * msPerHour
	fromDay := nowMs/msPerDay - (dimDayWindow - 1)
	fromDayMs := fromDay * msPerDay

	if err := w.recomputeStats(tx, fromHour, fromHourMs); err != nil {
		return err
	}
	if err := w.recomputeDims(tx, fromDay, fromDayMs); err != nil {
		return err
	}
	return tx.Commit()
}

func (w *Worker) recomputeStats(tx *sql.Tx, fromHour, fromHourMs int64) error {
	if _, err := tx.Exec(`DELETE FROM rollup_stats WHERE bucket_hour >= ?`, fromHour); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO rollup_stats(website_id, bucket_hour, pageviews, visitors, sessions)
		SELECT website_id, ts/?,
		       SUM(CASE WHEN type=0 THEN 1 ELSE 0 END),
		       COUNT(DISTINCT visitor_hash),
		       0
		FROM event WHERE ts >= ?
		GROUP BY website_id, ts/?`,
		msPerHour, fromHourMs, msPerHour); err != nil {
		return err
	}
	// Sessions are counted by their start hour from the session table.
	if _, err := tx.Exec(`
		UPDATE rollup_stats SET sessions = COALESCE((
			SELECT COUNT(*) FROM session s
			WHERE s.website_id = rollup_stats.website_id
			  AND s.started_at / ? = rollup_stats.bucket_hour
		), 0)
		WHERE bucket_hour >= ?`,
		msPerHour, fromHour); err != nil {
		return err
	}
	return nil
}

func (w *Worker) recomputeDims(tx *sql.Tx, fromDay, fromDayMs int64) error {
	if _, err := tx.Exec(`DELETE FROM rollup_dim WHERE bucket_day >= ?`, fromDay); err != nil {
		return err
	}

	for _, d := range eventDims {
		typeFilter := ""
		if d.pageviewsOnly {
			typeFilter = "AND type = 0"
		}
		q := `
			INSERT INTO rollup_dim(website_id, bucket_day, dimension, value, pageviews, sessions, visitors)
			SELECT website_id, ts/?, ?, ` + d.column + `,
			       COUNT(*), COUNT(DISTINCT session_id), COUNT(DISTINCT visitor_hash)
			FROM event
			WHERE ts >= ? AND ` + d.column + ` <> '' ` + typeFilter + `
			GROUP BY website_id, ts/?, ` + d.column
		if _, err := tx.Exec(q, msPerDay, d.name, fromDayMs, msPerDay); err != nil {
			return err
		}
	}

	// Custom events (by name).
	if _, err := tx.Exec(`
		INSERT INTO rollup_dim(website_id, bucket_day, dimension, value, pageviews, sessions, visitors)
		SELECT website_id, ts/?, 'event', name,
		       COUNT(*), COUNT(DISTINCT session_id), COUNT(DISTINCT visitor_hash)
		FROM event
		WHERE ts >= ? AND type = 1 AND name <> ''
		GROUP BY website_id, ts/?, name`,
		msPerDay, fromDayMs, msPerDay); err != nil {
		return err
	}

	// Entry / exit pages from sessions (counted by session start day).
	for _, ee := range []struct{ dim, col string }{{"entry", "entry_path"}, {"exit", "exit_path"}} {
		q := `
			INSERT INTO rollup_dim(website_id, bucket_day, dimension, value, pageviews, sessions, visitors)
			SELECT website_id, started_at/?, ?, ` + ee.col + `,
			       COUNT(*), COUNT(*), COUNT(DISTINCT visitor_hash)
			FROM session
			WHERE started_at >= ? AND ` + ee.col + ` <> ''
			GROUP BY website_id, started_at/?, ` + ee.col
		if _, err := tx.Exec(q, msPerDay, ee.dim, fromDayMs, msPerDay); err != nil {
			return err
		}
	}
	return nil
}
