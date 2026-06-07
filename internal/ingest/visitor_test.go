package ingest

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/klppl/glimt/internal/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestVisitorHash(t *testing.T) {
	db := newTestDB(t)
	sm, err := NewSaltManager(db.W)
	if err != nil {
		t.Fatal(err)
	}

	const ua = "Mozilla/5.0"
	base := sm.Hash(1, "203.0.113.5", ua)

	// Deterministic for identical inputs within the same day.
	if !bytes.Equal(base, sm.Hash(1, "203.0.113.5", ua)) {
		t.Error("hash not stable for identical inputs")
	}
	// Different IP -> different hash.
	if bytes.Equal(base, sm.Hash(1, "203.0.113.6", ua)) {
		t.Error("different IP produced same hash")
	}
	// Different website -> different hash (tenant isolation).
	if bytes.Equal(base, sm.Hash(2, "203.0.113.5", ua)) {
		t.Error("different website produced same hash")
	}
	// Different UA -> different hash.
	if bytes.Equal(base, sm.Hash(1, "203.0.113.5", "Other")) {
		t.Error("different UA produced same hash")
	}
	// Hash length is sha256.
	if len(base) != 32 {
		t.Errorf("hash length = %d, want 32", len(base))
	}
}

func TestSaltPersisted(t *testing.T) {
	db := newTestDB(t)
	sm, err := NewSaltManager(db.W)
	if err != nil {
		t.Fatal(err)
	}
	h1 := sm.Hash(1, "198.51.100.1", "ua")

	// A new manager on the same DB must reuse the persisted salt, so the same
	// visitor stays stable across a restart within the day.
	sm2, err := NewSaltManager(db.W)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(h1, sm2.Hash(1, "198.51.100.1", "ua")) {
		t.Error("salt not persisted across manager restart")
	}

	var n int
	if err := db.W.QueryRow("SELECT COUNT(*) FROM salt").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("salt rows = %d, want 1", n)
	}
}
