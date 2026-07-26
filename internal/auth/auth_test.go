package auth

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/klppl/glimt/internal/store"
)

func TestMultiUserAuth(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	a := New(db, 24*time.Hour, false)
	if err := a.EnsureAdmin("admin", "secret123"); err != nil {
		t.Fatalf("EnsureAdmin failed: %v", err)
	}

	// List users
	users, err := a.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("ListUsers len = %d, want 1", len(users))
	}
	if users[0].Username != "admin" || users[0].Role != "admin" {
		t.Errorf("User mismatched: %+v", users[0])
	}

	// Create user
	if err := a.CreateUser("editor", "pass456", "member"); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	users, _ = a.ListUsers()
	if len(users) != 2 {
		t.Fatalf("ListUsers after create len = %d, want 2", len(users))
	}

	// Login with new user
	tok, err := a.Login("editor", "pass456")
	if err != nil || tok == "" {
		t.Fatalf("Login with new user failed: %v", err)
	}
	if !a.valid(tok) {
		t.Errorf("Session token invalid")
	}

	// Delete user
	if err := a.DeleteUser(users[1].ID); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}
	users, _ = a.ListUsers()
	if len(users) != 1 {
		t.Fatalf("ListUsers after delete len = %d, want 1", len(users))
	}
}
