package ingest

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"sync"
	"time"
)

// SaltManager owns the daily-rotating salt used to derive cookieless visitor
// hashes. The salt is persisted (current + previous day) so a restart does not
// recount uniques mid-day; salts older than yesterday are deleted, making
// hashes unlinkable across days and irreversible to an IP.
type SaltManager struct {
	db *sql.DB

	mu   sync.RWMutex
	day  string
	salt []byte
}

func NewSaltManager(db *sql.DB) (*SaltManager, error) {
	sm := &SaltManager{db: db}
	if err := sm.ensure(time.Now().UTC()); err != nil {
		return nil, err
	}
	return sm, nil
}

func (sm *SaltManager) ensure(now time.Time) error {
	day := now.Format("2006-01-02")

	sm.mu.RLock()
	ok := sm.day == day && sm.salt != nil
	sm.mu.RUnlock()
	if ok {
		return nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.day == day && sm.salt != nil {
		return nil
	}

	var salt []byte
	err := sm.db.QueryRow("SELECT value FROM salt WHERE day = ?", day).Scan(&salt)
	if err == sql.ErrNoRows {
		salt = make([]byte, 32)
		if _, err := rand.Read(salt); err != nil {
			return err
		}
		if _, err := sm.db.Exec(
			"INSERT OR IGNORE INTO salt(day, value, created_at) VALUES(?,?,?)",
			day, salt, now.UnixMilli()); err != nil {
			return err
		}
		// Re-read to win any race where a concurrent insert got there first.
		if err := sm.db.QueryRow("SELECT value FROM salt WHERE day = ?", day).Scan(&salt); err != nil {
			return err
		}
		yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
		_, _ = sm.db.Exec("DELETE FROM salt WHERE day <> ? AND day <> ?", day, yesterday)
	} else if err != nil {
		return err
	}

	sm.day = day
	sm.salt = salt
	return nil
}

// Hash derives the daily visitor hash. The IP is used only here and never stored.
func (sm *SaltManager) Hash(websiteID int64, ip, userAgent string) []byte {
	_ = sm.ensure(time.Now().UTC())

	sm.mu.RLock()
	salt := sm.salt
	sm.mu.RUnlock()

	h := sha256.New()
	h.Write(salt)
	var idb [8]byte
	binary.BigEndian.PutUint64(idb[:], uint64(websiteID))
	h.Write(idb[:])
	h.Write([]byte(ip))
	h.Write([]byte(userAgent))
	return h.Sum(nil)
}
