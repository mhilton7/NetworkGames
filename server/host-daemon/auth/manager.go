package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	passwordSchema = 1
	argonTime      = 3
	argonMemory    = 64 * 1024
	argonThreads   = 2
	argonKeyLength = 32
)

type passwordRecord struct {
	Schema  int    `json:"schema"`
	Salt    string `json:"salt"`
	Hash    string `json:"hash"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory_kib"`
	Threads uint8  `json:"threads"`
	KeyLen  uint32 `json:"key_length"`
}

type Session struct {
	ID             string
	CSRF           string
	Expires        time.Time
	PasswordChange bool
}

type Manager struct {
	mu        sync.Mutex
	root      string
	username  string
	bootstrap string
	ttl       time.Duration
	sessions  map[string]Session
	now       func() time.Time
}

func New(root, username, bootstrap string, ttl time.Duration) (*Manager, error) {
	if username == "" || len(bootstrap) < 8 {
		return nil, errors.New("invalid browser bootstrap credentials")
	}
	if ttl <= 0 || ttl > 7*24*time.Hour {
		return nil, errors.New("invalid browser session lifetime")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return nil, err
	}
	return &Manager{
		root: root, username: username, bootstrap: bootstrap, ttl: ttl,
		sessions: make(map[string]Session), now: time.Now,
	}, nil
}

func (m *Manager) passwordPath() string { return filepath.Join(m.root, "password.json") }

func (m *Manager) DefaultActive() bool {
	_, err := os.Stat(m.passwordPath())
	return errors.Is(err, os.ErrNotExist)
}

func (m *Manager) Authenticate(username, password string) (bool, bool) {
	if subtle.ConstantTimeCompare([]byte(username), []byte(m.username)) != 1 {
		return false, false
	}
	record, err := m.load()
	if errors.Is(err, os.ErrNotExist) {
		ok := subtle.ConstantTimeCompare([]byte(password), []byte(m.bootstrap)) == 1
		return ok, ok
	}
	if err != nil {
		return false, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(record.Salt)
	if err != nil {
		return false, false
	}
	expected, err := base64.RawStdEncoding.DecodeString(record.Hash)
	if err != nil {
		return false, false
	}
	actual := argon2.IDKey([]byte(password), salt, record.Time, record.Memory,
		record.Threads, record.KeyLen)
	return subtle.ConstantTimeCompare(actual, expected) == 1, false
}

func (m *Manager) NewSession(forcePasswordChange bool) (Session, error) {
	id, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	session := Session{
		ID: id, CSRF: csrf, Expires: m.now().Add(m.ttl),
		PasswordChange: forcePasswordChange,
	}
	m.sessions[id] = session
	return session, nil
}

func (m *Manager) Validate(id string) (Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	session, ok := m.sessions[id]
	return session, ok
}

func (m *Manager) Logout(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

func (m *Manager) ChangePassword(sessionID, current, next string) error {
	if len(next) < 12 {
		return errors.New("new password must contain at least 12 characters")
	}
	session, ok := m.Validate(sessionID)
	if !ok {
		return errors.New("session expired")
	}
	valid, _ := m.Authenticate(m.username, current)
	if !valid {
		return errors.New("current password is incorrect")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key := argon2.IDKey([]byte(next), salt, argonTime, argonMemory, argonThreads, argonKeyLength)
	record := passwordRecord{
		Schema: passwordSchema,
		Salt:   base64.RawStdEncoding.EncodeToString(salt),
		Hash:   base64.RawStdEncoding.EncodeToString(key),
		Time:   argonTime, Memory: argonMemory, Threads: argonThreads, KeyLen: argonKeyLength,
	}
	if err := m.save(record); err != nil {
		return err
	}
	m.mu.Lock()
	m.sessions = make(map[string]Session)
	m.mu.Unlock()
	_ = session
	return nil
}

func (m *Manager) expireLocked() {
	now := m.now()
	for id, session := range m.sessions {
		if !now.Before(session.Expires) {
			delete(m.sessions, id)
		}
	}
}

func (m *Manager) load() (passwordRecord, error) {
	data, err := os.ReadFile(m.passwordPath())
	if err != nil {
		return passwordRecord{}, err
	}
	var record passwordRecord
	if err = json.Unmarshal(data, &record); err != nil {
		return passwordRecord{}, err
	}
	if record.Schema != passwordSchema || record.Time == 0 || record.Memory < 8*1024 ||
		record.Threads == 0 || record.KeyLen < 16 {
		return passwordRecord{}, errors.New("invalid browser password record")
	}
	return record, nil
}

func (m *Manager) save(record passwordRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(m.root, ".password-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0o600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, m.passwordPath()); err != nil {
		return err
	}
	directory, err := os.Open(m.root)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func randomToken(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
