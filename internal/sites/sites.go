// Package sites manages tracked websites (tenants) and an in-memory registry
// for fast token lookups on the ingest hot path.
package sites

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/klppl/glimt/internal/model"
	"github.com/klppl/glimt/internal/store"
)

type Registry struct {
	db *store.DB

	mu        sync.RWMutex
	byCollect map[string]*model.Site
	byScript  map[string]*model.Site
	byID      map[int64]*model.Site
	byShare   map[string]*model.Site
	ordered   []*model.Site
}

func New(db *store.DB) (*Registry, error) {
	r := &Registry{db: db}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Registry) Reload() error {
	rows, err := r.db.R.Query(
		`SELECT id, uuid, name, domain, COALESCE(api_key_hash,''), script_token,
		        collect_token, public, COALESCE(share_token,''), created_at
		 FROM website ORDER BY name`)
	if err != nil {
		return err
	}
	defer rows.Close()

	byCollect := map[string]*model.Site{}
	byScript := map[string]*model.Site{}
	byID := map[int64]*model.Site{}
	byShare := map[string]*model.Site{}
	var ordered []*model.Site

	for rows.Next() {
		s := &model.Site{}
		var public int
		if err := rows.Scan(&s.ID, &s.UUID, &s.Name, &s.Domain, &s.APIKeyHash,
			&s.ScriptToken, &s.CollectToken, &public, &s.ShareToken, &s.CreatedAt); err != nil {
			return err
		}
		s.Public = public != 0
		byCollect[s.CollectToken] = s
		byScript[s.ScriptToken] = s
		byID[s.ID] = s
		if s.ShareToken != "" {
			byShare[s.ShareToken] = s
		}
		ordered = append(ordered, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	r.byCollect, r.byScript, r.byID, r.byShare, r.ordered = byCollect, byScript, byID, byShare, ordered
	r.mu.Unlock()
	return nil
}

func (r *Registry) ByCollect(tok string) (*model.Site, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byCollect[tok]
	return s, ok
}

func (r *Registry) ByScript(tok string) (*model.Site, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byScript[tok]
	return s, ok
}

func (r *Registry) ByID(id int64) (*model.Site, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byID[id]
	return s, ok
}

func (r *Registry) ByShare(tok string) (*model.Site, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byShare[tok]
	return s, ok
}

func (r *Registry) All() []*model.Site {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*model.Site, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// Create inserts a new site with freshly generated random tokens and returns
// the plaintext API key once (only its hash is stored).
func (r *Registry) Create(name, domain string) (*model.Site, string, error) {
	apiKey := token(24)
	s := &model.Site{
		UUID:         token(16),
		Name:         name,
		Domain:       domain,
		APIKeyHash:   hashKey(apiKey),
		ScriptToken:  token(9),
		CollectToken: token(9),
		CreatedAt:    time.Now().UnixMilli(),
	}
	res, err := r.db.W.Exec(
		`INSERT INTO website(uuid, name, domain, api_key_hash, script_token, collect_token, public, created_at)
		 VALUES(?,?,?,?,?,?,0,?)`,
		s.UUID, s.Name, s.Domain, s.APIKeyHash, s.ScriptToken, s.CollectToken, s.CreatedAt)
	if err != nil {
		return nil, "", err
	}
	if s.ID, err = res.LastInsertId(); err != nil {
		return nil, "", err
	}
	return s, apiKey, r.Reload()
}

func (r *Registry) Update(id int64, name, domain string) error {
	if _, err := r.db.W.Exec(
		`UPDATE website SET name = ?, domain = ? WHERE id = ?`, name, domain, id); err != nil {
		return err
	}
	return r.Reload()
}

func (r *Registry) Delete(id int64) error {
	if _, err := r.db.W.Exec(`DELETE FROM website WHERE id = ?`, id); err != nil {
		return err
	}
	return r.Reload()
}

// RegenTokens rotates the script and collect paths (e.g. if a list ever catches up).
func (r *Registry) RegenTokens(id int64) error {
	if _, err := r.db.W.Exec(
		`UPDATE website SET script_token = ?, collect_token = ? WHERE id = ?`,
		token(9), token(9), id); err != nil {
		return err
	}
	return r.Reload()
}

// SetPublic toggles the public share dashboard, minting a share token on enable.
func (r *Registry) SetPublic(id int64, public bool) error {
	if public {
		if _, err := r.db.W.Exec(
			`UPDATE website SET public = 1, share_token = COALESCE(NULLIF(share_token,''), ?) WHERE id = ?`,
			token(12), id); err != nil {
			return err
		}
	} else {
		if _, err := r.db.W.Exec(`UPDATE website SET public = 0 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return r.Reload()
}

func token(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return hex.EncodeToString(b)
}

func hashKey(k string) string {
	sum := sha256.Sum256([]byte(k))
	return hex.EncodeToString(sum[:])
}
