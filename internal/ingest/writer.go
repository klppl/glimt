package ingest

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/klppl/glimt/internal/geo"
	"github.com/klppl/glimt/internal/model"
	"github.com/klppl/glimt/internal/store"
)

const (
	queueSize    = 8192
	batchSize    = 500
	flushEvery   = time.Second
	sessionWinMs = 30 * 60 * 1000 // 30-minute inactivity window
)

// Ingestor owns the buffered write path: events are enqueued by the hot HTTP
// handler and flushed to SQLite in batched transactions by a single goroutine.
type Ingestor struct {
	db   *store.DB
	salt *SaltManager
	geo  *geo.Geo

	ch      chan *model.Event
	wg      sync.WaitGroup
	dropped uint64
}

func New(db *store.DB, salt *SaltManager, g *geo.Geo) *Ingestor {
	return &Ingestor{
		db:   db,
		salt: salt,
		geo:  g,
		ch:   make(chan *model.Event, queueSize),
	}
}

func (in *Ingestor) Start() {
	in.wg.Add(1)
	go in.loop()
}

// Enqueue is non-blocking; under pathological burst it drops rather than stall
// the hot path. Returns false when the event was dropped.
func (in *Ingestor) Enqueue(ev *model.Event) bool {
	select {
	case in.ch <- ev:
		return true
	default:
		in.dropped++
		if in.dropped%1000 == 1 {
			log.Printf("ingest: queue full, dropped %d events", in.dropped)
		}
		return false
	}
}

func (in *Ingestor) Stop() {
	close(in.ch)
	in.wg.Wait()
}

func (in *Ingestor) loop() {
	defer in.wg.Done()
	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()

	batch := make([]*model.Event, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := in.flush(batch); err != nil {
			log.Printf("ingest: flush failed: %v", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case ev, ok := <-in.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, ev)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (in *Ingestor) flush(batch []*model.Event) error {
	tx, err := in.db.W.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	for _, ev := range batch {
		if err := insertEvent(tx, ev); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// insertEvent stitches the event into an open session (or opens one) and writes
// the event plus any custom props. Runs inside the writer's single txn.
func insertEvent(tx *sql.Tx, ev *model.Event) error {
	day := time.UnixMilli(ev.TS).UTC().Format("2006-01-02")

	var sid int64
	var pageviews int
	err := tx.QueryRow(
		`SELECT id, pageviews FROM session
		 WHERE website_id = ? AND visitor_hash = ? AND last_seen_at > ?
		 ORDER BY last_seen_at DESC LIMIT 1`,
		ev.WebsiteID, ev.VisitorHash, ev.TS-sessionWinMs).Scan(&sid, &pageviews)

	switch {
	case err == sql.ErrNoRows:
		pv := 0
		if ev.Type == model.TypePageview {
			pv = 1
		}
		res, err := tx.Exec(
			`INSERT INTO session
			 (website_id, visitor_hash, day, started_at, last_seen_at, entry_path, exit_path,
			  pageviews, events, is_bounce, country, browser, os, device, ref_class, ref_source, city)
			 VALUES (?,?,?,?,?,?,?,?,?,1,?,?,?,?,?,?,?)`,
			ev.WebsiteID, ev.VisitorHash, day, ev.TS, ev.TS, ev.URLPath, ev.URLPath,
			pv, 1, ev.Country, ev.Browser, ev.OS, ev.Device, ev.RefClass, ev.RefSource, ev.City)
		if err != nil {
			return err
		}
		if sid, err = res.LastInsertId(); err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		if ev.Type == model.TypePageview {
			newPV := pageviews + 1
			bounce := 0
			if newPV <= 1 {
				bounce = 1
			}
			if _, err := tx.Exec(
				`UPDATE session SET last_seen_at = ?, exit_path = ?, pageviews = ?,
				 events = events + 1, is_bounce = ? WHERE id = ?`,
				ev.TS, ev.URLPath, newPV, bounce, sid); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(
				`UPDATE session SET last_seen_at = ?, events = events + 1 WHERE id = ?`,
				ev.TS, sid); err != nil {
				return err
			}
		}
	}

	res, err := tx.Exec(
		`INSERT INTO event
		 (website_id, session_id, visitor_hash, ts, type, name, url_path, page_title, hostname, referrer,
		  ref_class, ref_source, utm_source, utm_medium, utm_campaign, utm_term, utm_content,
		  browser, browser_ver, os, device, screen_bucket, language, country, region, city,
		  lcp, inp, cls, ttfb, val)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ev.WebsiteID, sid, ev.VisitorHash, ev.TS, ev.Type, ev.Name, ev.URLPath, ev.PageTitle, ev.Hostname, ev.Referrer,
		ev.RefClass, ev.RefSource, ev.UTMSource, ev.UTMMedium, ev.UTMCampaign, ev.UTMTerm, ev.UTMContent,
		ev.Browser, ev.BrowserVer, ev.OS, ev.Device, ev.ScreenBucket, ev.Language, ev.Country, ev.Region, ev.City,
		ev.LCP, ev.INP, ev.CLS, ev.TTFB, ev.Val)
	if err != nil {
		return err
	}

	if len(ev.Props) > 0 {
		eid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for k, v := range ev.Props {
			if _, err := tx.Exec(
				`INSERT INTO event_prop(event_id, key, value) VALUES(?,?,?)`,
				eid, k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
