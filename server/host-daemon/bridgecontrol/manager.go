package bridgecontrol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	piManagementPort = "9443"
	statusInterval   = 10 * time.Second
	statusTimeout    = 4 * time.Second
)

// Manager owns one Pi client, one status poller, and a persisted IP override.
// Browser traffic reads the cache and can never increase polling frequency.
type Manager struct {
	mu              sync.RWMutex
	client          *Client
	token           string
	certificatePath string
	addressPath     string
	address         string
	status          Status
	statusErr       error
	wake            chan struct{}
}

func NewManager(baseURL, token, certificatePath, addressPath string) (*Manager, error) {
	if baseURL == "" && token == "" && certificatePath == "" {
		return nil, nil
	}
	if token == "" || certificatePath == "" {
		return nil, errors.New("Pi coordination requires token and pinned certificate")
	}
	address := ""
	if saved, err := os.ReadFile(addressPath); err == nil {
		address = strings.TrimSpace(string(saved))
		if net.ParseIP(address) == nil {
			return nil, errors.New("persisted Pi address is not an IP address")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read persisted Pi address: %w", err)
	}
	if address == "" {
		if baseURL != "" {
			parsed, err := url.Parse(baseURL)
			if err != nil || parsed.Hostname() == "" {
				return nil, errors.New("Pi management URL is invalid")
			}
			address = parsed.Hostname()
		}
	}
	var client *Client
	if address != "" {
		var err error
		client, err = New(managementURL(address), token, certificatePath)
		if err != nil {
			return nil, err
		}
	}
	return &Manager{
		client: client, token: token, certificatePath: certificatePath,
		addressPath: addressPath, address: address,
		statusErr: errors.New("Pi status has not been checked"),
		wake:      make(chan struct{}, 1),
	}, nil
}

func managementURL(address string) string {
	return "https://" + net.JoinHostPort(address, piManagementPort)
}

func (m *Manager) Action(ctx context.Context, action string) error {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		return errors.New("Pi address is not configured")
	}
	return client.Action(ctx, action)
}

// Status returns only the cached result. It never performs network I/O.
func (m *Manager) Status(context.Context) (Status, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status, m.statusErr
}

func (m *Manager) Address() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.address
}

func (m *Manager) SetAddress(_ context.Context, address string) error {
	address = strings.TrimSpace(address)
	if net.ParseIP(address) == nil {
		return errors.New("Pi address must be a literal IPv4 or IPv6 address")
	}
	client, err := New(managementURL(address), m.token, m.certificatePath)
	if err != nil {
		return err
	}
	if err := persistAddress(m.addressPath, address); err != nil {
		return err
	}
	m.mu.Lock()
	m.address = address
	m.client = client
	m.status = Status{}
	m.statusErr = errors.New("Pi status is pending")
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return nil
}

func (m *Manager) Run(ctx context.Context) {
	m.poll(ctx)
	ticker := time.NewTicker(statusInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.poll(ctx)
		case <-m.wake:
			m.poll(ctx)
		}
	}
}

func (m *Manager) poll(parent context.Context) {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		m.mu.Lock()
		if m.client == nil {
			m.status, m.statusErr = Status{}, errors.New("Pi address is not configured")
		}
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(parent, statusTimeout)
	status, err := client.Status(ctx)
	cancel()
	m.mu.Lock()
	// Do not publish a result from an address that was replaced mid-request.
	if client == m.client {
		m.status, m.statusErr = status, err
	}
	m.mu.Unlock()
}

func persistAddress(path, address string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pi-address-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.WriteString(address + "\n")
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
