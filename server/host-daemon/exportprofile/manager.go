// Package exportprofile serializes platform export transitions. It keeps Wii
// and GameCube behavior behind explicit profiles instead of platform checks in
// protocol and gadget code.
package exportprofile

import (
	"errors"
	"fmt"
	"sync"

	"wiibridge/server/nbd-plugin"
)

type State string

const (
	Disconnected     State = "DISCONNECTED"
	Preparing        State = "PREPARING"
	Validating       State = "VALIDATING"
	Exporting        State = "EXPORTING"
	Connected        State = "CONNECTED"
	Active           State = "ACTIVE"
	Disconnecting    State = "DISCONNECTING"
	RecoveryRequired State = "RECOVERY_REQUIRED"
	Error            State = "ERROR"
)

type Profile interface {
	Platform() string
	Backend() nbd.Backend
	ReadOnly() bool
	Validate() error
	Close() error
}

type BasicProfile struct {
	Name            string
	BlockBackend    nbd.Backend
	Immutable       bool
	ValidateProfile func() error
	CloseProfile    func() error
}

func (p *BasicProfile) Platform() string     { return p.Name }
func (p *BasicProfile) Backend() nbd.Backend { return p.BlockBackend }
func (p *BasicProfile) ReadOnly() bool       { return p.Immutable }
func (p *BasicProfile) Validate() error {
	if p.Name != "wii" && p.Name != "gamecube" {
		return errors.New("unsupported export platform")
	}
	if p.BlockBackend == nil {
		return errors.New("export backend is missing")
	}
	if p.ValidateProfile != nil {
		return p.ValidateProfile()
	}
	return nil
}
func (p *BasicProfile) Close() error {
	if p.CloseProfile != nil {
		return p.CloseProfile()
	}
	return nil
}

type Manager struct {
	mu       sync.Mutex
	state    State
	current  Profile
	sessions int
}

func New(defaultProfile Profile) (*Manager, error) {
	if defaultProfile == nil || defaultProfile.Platform() != "wii" ||
		!defaultProfile.ReadOnly() {
		return nil, errors.New("default profile must be immutable Wii mode")
	}
	if err := defaultProfile.Validate(); err != nil {
		return nil, err
	}
	return &Manager{state: Exporting, current: defaultProfile}, nil
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *Manager) Platform() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return ""
	}
	return m.current.Platform()
}

func (m *Manager) BeginSession() (nbd.Backend, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch m.state {
	case Exporting, Connected, Active:
	default:
		return nil, nil, fmt.Errorf("cannot begin I/O session in %s", m.state)
	}
	if m.current == nil {
		return nil, nil, errors.New("no active export profile")
	}
	m.sessions++
	m.state = Active
	released := false
	return m.current.Backend(), func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if released {
			return
		}
		released = true
		m.sessions--
		if m.sessions == 0 && m.state == Active {
			m.state = Connected
		}
	}, nil
}

func (m *Manager) MarkConnected() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != Exporting {
		return fmt.Errorf("cannot mark connected from %s", m.state)
	}
	m.state = Connected
	return nil
}

func (m *Manager) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions != 0 {
		return errors.New("cannot disconnect while I/O sessions are active")
	}
	switch m.state {
	case Exporting, Connected:
		m.state = Disconnecting
		m.state = Disconnected
		return nil
	case Disconnected:
		return nil
	default:
		return fmt.Errorf("cannot disconnect from %s", m.state)
	}
}

func (m *Manager) Select(next Profile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != Disconnected || m.sessions != 0 {
		return fmt.Errorf("profile selection requires DISCONNECTED with no active I/O; state=%s sessions=%d",
			m.state, m.sessions)
	}
	if next == nil {
		return errors.New("next profile is nil")
	}
	m.state = Preparing
	m.state = Validating
	if err := next.Validate(); err != nil {
		m.state = Error
		return err
	}
	previous := m.current
	m.current = next
	m.state = Exporting
	if previous != nil && previous != next {
		if err := previous.Close(); err != nil {
			m.state = RecoveryRequired
			return fmt.Errorf("previous profile cleanup: %w", err)
		}
	}
	return nil
}

func (m *Manager) Recover() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != Error && m.state != RecoveryRequired {
		return fmt.Errorf("recovery is invalid from %s", m.state)
	}
	if m.sessions != 0 {
		return errors.New("cannot recover with active sessions")
	}
	m.state = Disconnected
	return nil
}
