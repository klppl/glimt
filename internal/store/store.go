// Package store opens the SQLite database with two pools: a single-writer pool
// (SQLite serializes writes anyway) and a read pool for the dashboard, plus an
// embedded, versioned migration runner.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type DB struct {
	W *sql.DB // single-writer (ingest + rollup + admin writes)
	R *sql.DB // read pool (dashboard queries)
}

func Open(path string) (*DB, error) {
	wdsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(1)&_pragma=foreign_keys(1)"
	rdsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"

	w, err := sql.Open("sqlite", wdsn)
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)

	r, err := sql.Open("sqlite", rdsn)
	if err != nil {
		_ = w.Close()
		return nil, err
	}
	r.SetMaxOpenConns(8)

	db := &DB{W: w, R: r}
	if err := db.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	if db.W != nil {
		_ = db.W.Close()
	}
	if db.R != nil {
		_ = db.R.Close()
	}
	return nil
}

// migrate applies any embedded migration whose 1-based ordinal exceeds the
// stored PRAGMA user_version. Files use IF NOT EXISTS so re-running is safe.
func (db *DB) migrate() error {
	var ver int
	if err := db.W.QueryRow("PRAGMA user_version").Scan(&ver); err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for i, f := range files {
		target := i + 1
		if target <= ver {
			continue
		}
		b, err := migrationsFS.ReadFile("migrations/" + f)
		if err != nil {
			return err
		}
		if _, err := db.W.Exec(string(b)); err != nil {
			return fmt.Errorf("migration %s: %w", f, err)
		}
		if _, err := db.W.Exec(fmt.Sprintf("PRAGMA user_version = %d", target)); err != nil {
			return err
		}
	}
	return nil
}
