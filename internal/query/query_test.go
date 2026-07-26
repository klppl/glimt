package query

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/klppl/glimt/internal/model"
	"github.com/klppl/glimt/internal/store"
)

func TestTier1Queries(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Insert test site
	res, err := db.W.Exec(`INSERT INTO website(uuid, name, domain, script_token, collect_token, created_at) VALUES('u1', 'Site 1', 'example.com', 'st1', 'ct1', 1000)`)
	if err != nil {
		t.Fatalf("Insert site failed: %v", err)
	}
	siteID, _ := res.LastInsertId()

	now := time.Now().UnixMilli()
	day := time.UnixMilli(now).UTC().Format("2006-01-02")

	// Insert session
	sres, err := db.W.Exec(`INSERT INTO session(website_id, visitor_hash, day, started_at, last_seen_at, entry_path, exit_path, pageviews, events, is_bounce) VALUES(?, X'01020304', ?, ?, ?, '/', '/', 1, 1, 0)`, siteID, day, now-1000, now)
	if err != nil {
		t.Fatalf("Insert session failed: %v", err)
	}
	sid, _ := sres.LastInsertId()

	// Insert events with Tier 1 data
	_, err = db.W.Exec(`INSERT INTO event(website_id, session_id, visitor_hash, ts, type, name, url_path, page_title, hostname, referrer, browser, os, device, country, language, lcp, inp, cls, ttfb, val) VALUES(?, ?, X'01020304', ?, ?, 'purchase', '/checkout', 'Checkout Page', 'example.com', '', 'Chrome', 'Linux', 'desktop', 'US', 'en', 1500.0, 120.0, 0.02, 200.0, 49.99)`, siteID, sid, now, model.TypeCustom)
	if err != nil {
		t.Fatalf("Insert event failed: %v", err)
	}

	q := New(db)
	fromMs := now - 3600*1000
	toMs := now + 3600*1000

	// 1. Test Summary (including Revenue)
	sum, err := q.Summary(siteID, fromMs, toMs)
	if err != nil {
		t.Fatalf("Summary failed: %v", err)
	}
	if sum.Revenue != 49.99 {
		t.Errorf("Revenue = %f, want 49.99", sum.Revenue)
	}

	// 2. Test Vitals
	vitals, err := q.Vitals(siteID, fromMs, toMs)
	if err != nil {
		t.Fatalf("Vitals failed: %v", err)
	}
	if vitals.Count != 1 {
		t.Errorf("Vitals count = %d, want 1", vitals.Count)
	}
	if vitals.LCP != 1500.0 || vitals.INP != 120.0 || vitals.CLS != 0.02 || vitals.TTFB != 200.0 {
		t.Errorf("Vitals mismatched: %+v", vitals)
	}

	// 3. Test ExportEvents
	events, err := q.ExportEvents(siteID, fromMs, toMs)
	if err != nil {
		t.Fatalf("ExportEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("ExportEvents len = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.PageTitle != "Checkout Page" || ev.Hostname != "example.com" || ev.Val != 49.99 {
		t.Errorf("ExportEvent mismatched: %+v", ev)
	}
}

func TestFunnelQueries(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	q := New(db)
	now := time.Now().UnixMilli()

	// Insert events for session 1: step 1 (/) -> step 2 (/pricing) -> step 3 (signup)
	_, _ = db.W.Exec(`INSERT INTO event(website_id, session_id, visitor_hash, ts, type, name, url_path) VALUES(1, 10, X'01', ?, 0, '', '/')`, now-100)
	_, _ = db.W.Exec(`INSERT INTO event(website_id, session_id, visitor_hash, ts, type, name, url_path) VALUES(1, 10, X'01', ?, 0, '', '/pricing')`, now-50)
	_, _ = db.W.Exec(`INSERT INTO event(website_id, session_id, visitor_hash, ts, type, name, url_path) VALUES(1, 10, X'01', ?, 1, 'signup', '')`, now)

	// Insert events for session 2: step 1 (/) -> step 2 (/pricing) (no signup)
	_, _ = db.W.Exec(`INSERT INTO event(website_id, session_id, visitor_hash, ts, type, name, url_path) VALUES(1, 11, X'02', ?, 0, '', '/')`, now-100)
	_, _ = db.W.Exec(`INSERT INTO event(website_id, session_id, visitor_hash, ts, type, name, url_path) VALUES(1, 11, X'02', ?, 0, '', '/pricing')`, now-50)

	steps := []FunnelStep{
		{Name: "/", IsURL: true},
		{Name: "/pricing", IsURL: true},
		{Name: "signup", IsURL: false},
	}

	fn, err := q.CreateFunnel(1, "Checkout Funnel", steps)
	if err != nil {
		t.Fatalf("CreateFunnel failed: %v", err)
	}
	if fn.Name != "Checkout Funnel" {
		t.Errorf("Funnel name = %q", fn.Name)
	}

	results, err := q.EvaluateFunnel(1, steps, now-3600*1000, now+3600*1000)
	if err != nil {
		t.Fatalf("EvaluateFunnel failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("EvaluateFunnel len = %d, want 3", len(results))
	}
	if results[0].Count != 2 || results[1].Count != 2 || results[2].Count != 1 {
		t.Errorf("Funnel step counts mismatched: %+v", results)
	}
	if results[2].Percentage != 50.0 {
		t.Errorf("Final conversion = %f, want 50.0", results[2].Percentage)
	}
}
