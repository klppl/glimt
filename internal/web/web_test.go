package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/klppl/glimt/internal/auth"
	"github.com/klppl/glimt/internal/dashboard"
	"github.com/klppl/glimt/internal/geo"
	"github.com/klppl/glimt/internal/ingest"
	"github.com/klppl/glimt/internal/query"
	"github.com/klppl/glimt/internal/sites"
	"github.com/klppl/glimt/internal/store"
	"github.com/klppl/glimt/internal/tracker"
)

func TestPixelAndExportRoutes(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	reg, err := sites.New(db)
	if err != nil {
		t.Fatalf("sites.New failed: %v", err)
	}
	s, _, err := reg.Create("Test Site", "example.com")
	if err != nil {
		t.Fatalf("Create site failed: %v", err)
	}

	sm, err := ingest.NewSaltManager(db.W)
	if err != nil {
		t.Fatalf("NewSaltManager failed: %v", err)
	}
	g, err := geo.Open("")
	if err != nil {
		t.Fatalf("geo.Open failed: %v", err)
	}
	defer g.Close()

	ing := ingest.New(db, sm, g)
	col := ingest.NewCollector(reg, ing, "", nil, false)
	trk := tracker.New(reg, "glimt", "")
	q := query.New(db)
	a := auth.New(db, 24*time.Hour, false)
	dash, err := dashboard.New(reg, q, a, dashboard.Config{})
	if err != nil {
		t.Fatalf("dashboard New failed: %v", err)
	}

	router := Router(col, trk, dash, a)

	// 1. Test Pixel endpoint
	req := httptest.NewRequest("GET", "/pixel/"+s.CollectToken+".gif?u=https://example.com/email&t=Email%20Open", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Pixel status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "image/gif" {
		t.Errorf("Pixel Content-Type = %q, want image/gif", rec.Header().Get("Content-Type"))
	}

	// 2. Test Export endpoint (auth gated, should redirect without auth)
	expReq := httptest.NewRequest("GET", "/app/export?site=1&format=json", nil)
	expRec := httptest.NewRecorder()
	router.ServeHTTP(expRec, expReq)
	if expRec.Code != http.StatusSeeOther {
		t.Errorf("Export unauthenticated status = %d, want 303 redirect", expRec.Code)
	}
}
