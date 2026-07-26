// Package auth provides single-admin dashboard authentication: pbkdf2 password
// hashing (stdlib crypto/pbkdf2) and opaque server-side session tokens stored
// in a first-party admin cookie.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/klppl/glimt/internal/store"
)

const (
	CookieName  = "glimt_session"
	pbkdfIter   = 120_000
	pbkdfKeyLen = 32
)

var ErrInvalid = errors.New("invalid credentials")

type Auth struct {
	db     *store.DB
	ttl    time.Duration
	secure bool
}

func New(db *store.DB, ttl time.Duration, secure bool) *Auth {
	return &Auth{db: db, ttl: ttl, secure: secure}
}

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// EnsureAdmin creates the admin account if none exists yet. If an admin already
// exists and a password is supplied, it is updated (lets you reset via env).
func (a *Auth) EnsureAdmin(username, password string) error {
	var n int
	if err := a.db.W.QueryRow(`SELECT COUNT(*) FROM admin`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if username == "" || password == "" {
			return errors.New("no admin exists and GLIMT_ADMIN_USER/GLIMT_ADMIN_PASS are not set")
		}
		_, err := a.db.W.Exec(`INSERT INTO admin(username, pw_hash, role) VALUES(?,?,'admin')`,
			username, hashPassword(password))
		return err
	}
	if username != "" && password != "" {
		_, err := a.db.W.Exec(`UPDATE admin SET pw_hash = ? WHERE username = ?`,
			hashPassword(password), username)
		return err
	}
	return nil
}

// Login verifies credentials and returns a fresh session token.
func (a *Auth) Login(username, password string) (string, error) {
	var uid int64
	var stored string
	err := a.db.R.QueryRow(`SELECT id, pw_hash FROM admin WHERE username = ?`, username).Scan(&uid, &stored)
	if err == sql.ErrNoRows {
		return "", ErrInvalid
	}
	if err != nil {
		return "", err
	}
	if !verifyPassword(stored, password) {
		return "", ErrInvalid
	}

	tok := randToken()
	expires := time.Now().Add(a.ttl).UnixMilli()
	if _, err := a.db.W.Exec(
		`INSERT INTO admin_session(token, expires_at, user_id) VALUES(?,?,?)`, tok, expires, uid); err != nil {
		return "", err
	}
	return tok, nil
}

func (a *Auth) ListUsers() ([]User, error) {
	rows, err := a.db.R.Query(`SELECT id, username, COALESCE(role, 'admin') FROM admin ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (a *Auth) CreateUser(username, password, role string) error {
	if username == "" || password == "" {
		return errors.New("username and password required")
	}
	if role == "" {
		role = "admin"
	}
	_, err := a.db.W.Exec(`INSERT INTO admin(username, pw_hash, role) VALUES(?,?,?)`,
		username, hashPassword(password), role)
	return err
}

func (a *Auth) DeleteUser(id int64) error {
	var count int
	_ = a.db.R.QueryRow(`SELECT COUNT(*) FROM admin`).Scan(&count)
	if count <= 1 {
		return errors.New("cannot delete the last admin user")
	}
	_, err := a.db.W.Exec(`DELETE FROM admin WHERE id = ?`, id)
	return err
}

func (a *Auth) Logout(token string) {
	if token == "" {
		return
	}
	_, _ = a.db.W.Exec(`DELETE FROM admin_session WHERE token = ?`, token)
}

func (a *Auth) valid(token string) bool {
	if token == "" {
		return false
	}
	var expires int64
	err := a.db.R.QueryRow(`SELECT expires_at FROM admin_session WHERE token = ?`, token).Scan(&expires)
	if err != nil {
		return false
	}
	if time.Now().UnixMilli() > expires {
		_, _ = a.db.W.Exec(`DELETE FROM admin_session WHERE token = ?`, token)
		return false
	}
	return true
}

// SetCookie writes the session cookie on a successful login.
func (a *Auth) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(a.ttl),
	})
}

func (a *Auth) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// Require is middleware that redirects unauthenticated requests to /login.
func (a *Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || !a.valid(c.Value) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func randToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashPassword(pw string) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	dk, err := pbkdf2.Key(sha256.New, pw, salt, pbkdfIter, pbkdfKeyLen)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("pbkdf2$%d$%s$%s",
		pbkdfIter,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(dk))
}

func verifyPassword(stored, pw string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}
