package rollup

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/klppl/glimt/internal/model"
	"github.com/klppl/glimt/internal/store"
)

func TestRetentionPruning(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	// Insert test site
	res, err := db.W.Exec(`INSERT INTO website(uuid, name, domain, script_token, collect_token, created_at) VALUES('u1', 'Site 1', 'example.com', 'st1', 'ct1', 1000)`)
	if err != nil {
		t.Fatalf("INSERT site failed: %v", err)
	}
	siteID, _ := res.LastInsertId()

	now := time.Now().UTC()

	// 10 days ago (old)
	oldTime := now.AddDate(0, 0, -10)
	oldMs := oldTime.UnixMilli()
	oldDay := oldTime.Format("2006-01-02")

	// 1 day ago (recent)
	recentTime := now.AddDate(0, 0, -1)
	recentMs := recentTime.UnixMilli()
	recentDay := recentTime.Format("2006-01-02")

	// Insert old session & event & event_prop
	sResOld, err := db.W.Exec(`INSERT INTO session(website_id, visitor_hash, day, started_at, last_seen_at, entry_path, exit_path, pageviews, events, is_bounce) VALUES(?, X'01', ?, ?, ?, '/', '/', 1, 1, 0)`, siteID, oldDay, oldMs-1000, oldMs)
	if err != nil {
		t.Fatalf("INSERT old session failed: %v", err)
	}
	oldSessionID, _ := sResOld.LastInsertId()

	eResOld, err := db.W.Exec(`INSERT INTO event(website_id, session_id, visitor_hash, ts, type, name, url_path, page_title) VALUES(?, ?, X'01', ?, ?, 'click', '/old', 'Old Page')`, siteID, oldSessionID, oldMs, model.TypeCustom)
	if err != nil {
		t.Fatalf("INSERT old event failed: %v", err)
	}
	oldEventID, _ := eResOld.LastInsertId()

	_, err = db.W.Exec(`INSERT INTO event_prop(event_id, key, value) VALUES(?, 'k1', 'v1')`, oldEventID)
	if err != nil {
		t.Fatalf("INSERT old event_prop failed: %v", err)
	}

	// Insert recent session & event & event_prop
	sResRecent, err := db.W.Exec(`INSERT INTO session(website_id, visitor_hash, day, started_at, last_seen_at, entry_path, exit_path, pageviews, events, is_bounce) VALUES(?, X'02', ?, ?, ?, '/', '/', 1, 1, 0)`, siteID, recentDay, recentMs-1000, recentMs)
	if err != nil {
		t.Fatalf("INSERT recent session failed: %v", err)
	}
	recentSessionID, _ := sResRecent.LastInsertId()

	eResRecent, err := db.W.Exec(`INSERT INTO event(website_id, session_id, visitor_hash, ts, type, name, url_path, page_title) VALUES(?, ?, X'02', ?, ?, 'click', '/recent', 'Recent Page')`, siteID, recentSessionID, recentMs, model.TypeCustom)
	if err != nil {
		t.Fatalf("INSERT recent event failed: %v", err)
	}
	recentEventID, _ := eResRecent.LastInsertId()

	_, err = db.W.Exec(`INSERT INTO event_prop(event_id, key, value) VALUES(?, 'k2', 'v2')`, recentEventID)
	if err != nil {
		t.Fatalf("INSERT recent event_prop failed: %v", err)
	}

	// Insert historical rollup_stats and rollup_dim (both old and recent)
	oldBucketHour := oldMs / msPerHour
	oldBucketDay := oldMs / msPerDay
	_, err = db.W.Exec(`INSERT INTO rollup_stats(website_id, bucket_hour, pageviews, visitors, sessions) VALUES(?, ?, 10, 5, 2)`, siteID, oldBucketHour)
	if err != nil {
		t.Fatalf("INSERT old rollup_stats failed: %v", err)
	}

	_, err = db.W.Exec(`INSERT INTO rollup_dim(website_id, bucket_day, dimension, value, pageviews, sessions, visitors) VALUES(?, ?, 'path', '/old', 10, 2, 5)`, siteID, oldBucketDay)
	if err != nil {
		t.Fatalf("INSERT old rollup_dim failed: %v", err)
	}

	// Retention set to 5 days
	w := New(db, 5)
	if err := w.recompute(now); err != nil {
		t.Fatalf("recompute failed: %v", err)
	}

	// Check event pruning
	var count int
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM event WHERE id = ?`, oldEventID).Scan(&count)
	if count != 0 {
		t.Errorf("old event count = %d, want 0 (pruned)", count)
	}
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM event WHERE id = ?`, recentEventID).Scan(&count)
	if count != 1 {
		t.Errorf("recent event count = %d, want 1 (kept)", count)
	}

	// Check event_prop pruning
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM event_prop WHERE event_id = ?`, oldEventID).Scan(&count)
	if count != 0 {
		t.Errorf("old event_prop count = %d, want 0 (pruned)", count)
	}
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM event_prop WHERE event_id = ?`, recentEventID).Scan(&count)
	if count != 1 {
		t.Errorf("recent event_prop count = %d, want 1 (kept)", count)
	}

	// Check session pruning
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM session WHERE id = ?`, oldSessionID).Scan(&count)
	if count != 0 {
		t.Errorf("old session count = %d, want 0 (pruned)", count)
	}
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM session WHERE id = ?`, recentSessionID).Scan(&count)
	if count != 1 {
		t.Errorf("recent session count = %d, want 1 (kept)", count)
	}

	// Check rollup tables were NOT pruned
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM rollup_stats WHERE bucket_hour = ?`, oldBucketHour).Scan(&count)
	if count != 1 {
		t.Errorf("old rollup_stats count = %d, want 1 (kept)", count)
	}
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM rollup_dim WHERE bucket_day = ? AND value = '/old'`, oldBucketDay).Scan(&count)
	if count != 1 {
		t.Errorf("old rollup_dim count = %d, want 1 (kept)", count)
	}
}

func TestRetentionDisabled(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	res, err := db.W.Exec(`INSERT INTO website(uuid, name, domain, script_token, collect_token, created_at) VALUES('u1', 'Site 1', 'example.com', 'st1', 'ct1', 1000)`)
	if err != nil {
		t.Fatalf("INSERT site failed: %v", err)
	}
	siteID, _ := res.LastInsertId()

	now := time.Now().UTC()
	oldTime := now.AddDate(0, 0, -100)
	oldMs := oldTime.UnixMilli()
	oldDay := oldTime.Format("2006-01-02")

	sRes, err := db.W.Exec(`INSERT INTO session(website_id, visitor_hash, day, started_at, last_seen_at, entry_path, exit_path, pageviews, events, is_bounce) VALUES(?, X'01', ?, ?, ?, '/', '/', 1, 1, 0)`, siteID, oldDay, oldMs-1000, oldMs)
	if err != nil {
		t.Fatalf("INSERT session failed: %v", err)
	}
	sid, _ := sRes.LastInsertId()

	eRes, err := db.W.Exec(`INSERT INTO event(website_id, session_id, visitor_hash, ts, type, name, url_path, page_title) VALUES(?, ?, X'01', ?, ?, 'click', '/old', 'Old Page')`, siteID, sid, oldMs, model.TypeCustom)
	if err != nil {
		t.Fatalf("INSERT event failed: %v", err)
	}
	eid, _ := eRes.LastInsertId()

	_, err = db.W.Exec(`INSERT INTO event_prop(event_id, key, value) VALUES(?, 'k1', 'v1')`, eid)
	if err != nil {
		t.Fatalf("INSERT event_prop failed: %v", err)
	}

	// retention = 0 (disabled)
	w := New(db, 0)
	if err := w.recompute(now); err != nil {
		t.Fatalf("recompute failed: %v", err)
	}

	var count int
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM event WHERE id = ?`, eid).Scan(&count)
	if count != 1 {
		t.Errorf("event count = %d, want 1 when retention is disabled", count)
	}
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM event_prop WHERE event_id = ?`, eid).Scan(&count)
	if count != 1 {
		t.Errorf("event_prop count = %d, want 1 when retention is disabled", count)
	}
	_ = db.R.QueryRow(`SELECT COUNT(*) FROM session WHERE id = ?`, sid).Scan(&count)
	if count != 1 {
		t.Errorf("session count = %d, want 1 when retention is disabled", count)
	}
}
