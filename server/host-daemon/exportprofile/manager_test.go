package exportprofile

import (
	"errors"
	"sync"
	"testing"
)

type testBackend struct{ size int64 }

func (b *testBackend) Size() int64                           { return b.size }
func (b *testBackend) ReadAt(p []byte, _ int64) (int, error) { return len(p), nil }

func profile(platform string, readOnly bool) *BasicProfile {
	return &BasicProfile{Name: platform, BlockBackend: &testBackend{size: 1 << 20}, Immutable: readOnly}
}

func TestWiiDefaultAndRoundTripProfileSwitch(t *testing.T) {
	manager, err := New(profile("wii", true))
	if err != nil {
		t.Fatal(err)
	}
	if manager.Platform() != "wii" || manager.State() != Exporting {
		t.Fatalf("Wii is not the default: platform=%s state=%s", manager.Platform(), manager.State())
	}
	if err = manager.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if err = manager.Select(profile("gamecube", false)); err != nil {
		t.Fatal(err)
	}
	if manager.Platform() != "gamecube" || manager.State() != Exporting {
		t.Fatalf("GameCube selection failed: platform=%s state=%s", manager.Platform(), manager.State())
	}
	if err = manager.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if err = manager.Select(profile("wii", true)); err != nil {
		t.Fatal(err)
	}
	if manager.Platform() != "wii" {
		t.Fatal("Wii profile was not restored")
	}
}

func TestSwitchFailsWithOutstandingRead(t *testing.T) {
	manager, err := New(profile("wii", true))
	if err != nil {
		t.Fatal(err)
	}
	_, release, err := manager.BeginSession()
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Disconnect(); err == nil {
		t.Fatal("disconnect accepted with an outstanding session")
	}
	if err = manager.Select(profile("gamecube", false)); err == nil {
		t.Fatal("selection accepted with an outstanding session")
	}
	release()
	if err = manager.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestFailedValidationRequiresExplicitRecovery(t *testing.T) {
	manager, err := New(profile("wii", true))
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Disconnect(); err != nil {
		t.Fatal(err)
	}
	bad := profile("gamecube", false)
	bad.ValidateProfile = func() error { return errors.New("invalid volume") }
	if err = manager.Select(bad); err == nil || manager.State() != Error {
		t.Fatalf("invalid volume state=%s err=%v", manager.State(), err)
	}
	if err = manager.Recover(); err != nil || manager.State() != Disconnected {
		t.Fatalf("recovery state=%s err=%v", manager.State(), err)
	}
}

func TestConcurrentSelectionSerializesAndAllowsOneWinner(t *testing.T) {
	manager, err := New(profile("wii", true))
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.Disconnect(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- manager.Select(profile("gamecube", false))
		}()
	}
	wg.Wait()
	close(results)
	var success int
	for err = range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent selection successes=%d, want 1", success)
	}
}

func TestDefaultRejectsWritableOrNonWiiProfile(t *testing.T) {
	if _, err := New(profile("gamecube", false)); err == nil {
		t.Fatal("GameCube became the implicit default")
	}
	if _, err := New(profile("wii", false)); err == nil {
		t.Fatal("writable Wii default accepted")
	}
}
