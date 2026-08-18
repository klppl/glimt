package ingest

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/klppl/glimt/internal/model"
	"github.com/klppl/glimt/internal/store"
)

func TestIngestFlush(t *testing.T) {
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

	in := New(db, nil, nil)
	now := time.Now().UnixMilli()

	events := []*model.Event{
		{
			WebsiteID:    siteID,
			VisitorHash:  []byte{1, 2, 3, 4},
			TS:           now,
			Type:         model.TypePageview,
			Name:         "",
			URLPath:      "/home",
			PageTitle:    "Home Page",
			Hostname:     "example.com",
			Referrer:     "https://google.com",
			RefClass:     model.RefSearch,
			RefSource:    "Google",
			UTMSource:    "newsletter",
			UTMMedium:    "email",
			UTMCampaign:  "spring",
			UTMTerm:      "shoes",
			UTMContent:   "hero_button",
			Browser:      "Chrome",
			BrowserVer:   "120.0",
			OS:           "macOS",
			Device:       "desktop",
			ScreenBucket: "1920x1080",
			Language:     "en-US",
			Country:      "US",
			Region:       "CA",
			City:         "San Francisco",
			LCP:          1200.5,
			INP:          45.2,
			CLS:          0.01,
			TTFB:         150.0,
			Val:          10.0,
			Props:        map[string]string{"plan": "pro"},
		},
	}

	if err := in.flush(events); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	var count int
	if err := db.R.QueryRow(`SELECT count(*) FROM event WHERE website_id = ?`, siteID).Scan(&count); err != nil {
		t.Fatalf("query event failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 event, got %d", count)
	}

	var sessionCount int
	if err := db.R.QueryRow(`SELECT count(*) FROM session WHERE website_id = ?`, siteID).Scan(&sessionCount); err != nil {
		t.Fatalf("query session failed: %v", err)
	}
	if sessionCount != 1 {
		t.Errorf("expected 1 session, got %d", sessionCount)
	}

	var propVal string
	if err := db.R.QueryRow(`SELECT value FROM event_prop WHERE key = 'plan'`).Scan(&propVal); err != nil {
		t.Fatalf("query event_prop failed: %v", err)
	}
	if propVal != "pro" {
		t.Errorf("expected prop value 'pro', got %q", propVal)
	}
}
