package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBootstrapPasswordChangePersistenceAndNoPlaintext(t *testing.T) {
	root := filepath.Join(t.TempDir(), "auth")
	manager, err := New(root, "admin", "wiibridge", 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ok, force := manager.Authenticate("admin", "wiibridge")
	if !ok || !force {
		t.Fatal("bootstrap login failed or did not force password change")
	}
	session, err := manager.NewSession(force)
	if err != nil {
		t.Fatal(err)
	}
	if !session.PasswordChange {
		t.Fatal("bootstrap session is not restricted")
	}
	next := "a-new-password-123"
	if err = manager.ChangePassword(session.ID, "wiibridge", next); err != nil {
		t.Fatal(err)
	}
	if _, valid := manager.Validate(session.ID); valid {
		t.Fatal("password change did not invalidate sessions")
	}
	data, err := os.ReadFile(filepath.Join(root, "password.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), next) || strings.Contains(string(data), "wiibridge") {
		t.Fatal("plaintext password persisted")
	}
	info, err := os.Stat(filepath.Join(root, "password.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("password mode=%v err=%v", info.Mode().Perm(), err)
	}
	restarted, err := New(root, "admin", "wiibridge", 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ = restarted.Authenticate("admin", next); !ok {
		t.Fatal("changed password did not survive restart")
	}
	if ok, _ = restarted.Authenticate("admin", "wiibridge"); ok {
		t.Fatal("bootstrap password remained active")
	}
}

func TestSessionExpirationLogoutAndWrongCredentials(t *testing.T) {
	manager, err := New(t.TempDir(), "admin", "wiibridge", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := manager.Authenticate("admin", "wrong"); ok {
		t.Fatal("wrong password accepted")
	}
	if ok, _ := manager.Authenticate("other", "wiibridge"); ok {
		t.Fatal("wrong username accepted")
	}
	now := time.Now()
	manager.now = func() time.Time { return now }
	session, err := manager.NewSession(false)
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, ok := manager.Validate(session.ID); ok {
		t.Fatal("expired session accepted")
	}
	manager.now = func() time.Time { return now }
	session, err = manager.NewSession(false)
	if err != nil {
		t.Fatal(err)
	}
	manager.Logout(session.ID)
	if _, ok := manager.Validate(session.ID); ok {
		t.Fatal("logged-out session accepted")
	}
}

func TestChangePasswordValidatesCurrentAndStrength(t *testing.T) {
	manager, err := New(t.TempDir(), "admin", "wiibridge", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	session, err := manager.NewSession(true)
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.ChangePassword(session.ID, "wrong", "long-enough-password"); err == nil {
		t.Fatal("wrong current password accepted")
	}
	if err = manager.ChangePassword(session.ID, "wiibridge", "short"); err == nil {
		t.Fatal("weak new password accepted")
	}
}
