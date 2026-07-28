package bridgecontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerPersistsOnlyValidatedIPAddress(t *testing.T) {
	certPEM, _ := certificate(t, "device")
	certPath := filepath.Join(t.TempDir(), "device.crt")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	addressPath := filepath.Join(t.TempDir(), "pi-address")
	manager, err := NewManager(
		"https://192.0.2.10:9443", "independent-pi-token", certPath, addressPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.SetAddress(context.Background(), "192.0.2.20"); err != nil {
		t.Fatal(err)
	}
	if manager.Address() != "192.0.2.20" {
		t.Fatalf("address = %q", manager.Address())
	}
	saved, err := os.ReadFile(addressPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != "192.0.2.20\n" {
		t.Fatalf("saved address = %q", saved)
	}
	info, err := os.Stat(addressPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("address mode = %o", info.Mode().Perm())
	}
	if err = manager.SetAddress(context.Background(), "https://attacker.invalid"); err == nil {
		t.Fatal("arbitrary URL was accepted as a Pi address")
	}
}

func TestManagerStatusIsCachedAndNeverPollsOnRequest(t *testing.T) {
	manager := &Manager{statusErr: os.ErrNotExist}
	if _, err := manager.Status(context.Background()); err == nil {
		t.Fatal("empty cached status was reported as connected")
	}
}

func TestManagerProbePerformsFreshAuthenticatedStatusRequest(t *testing.T) {
	const token = "independent-pi-token"
	certPEM, keyPEM := certificate(t, "device")
	certPath := filepath.Join(t.TempDir(), "device.crt")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if r.URL.Path != "/api/v1/status" || !ok || user != "admin" ||
			password != token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		requests++
		_ = json.NewEncoder(w).Encode(Status{
			Target: "zero-w-armhf", BoardOK: true, Provisioned: true,
			WiFiReady: true, USBController: "20980000.usb",
			USBState: "not attached", State: "ready",
		})
	}))
	server.TLS = serverTLS(t, certPEM, keyPEM)
	server.StartTLS()
	defer server.Close()
	client, err := New(server.URL, token, certPath)
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{
		client: client, statusErr: os.ErrNotExist,
	}
	status, err := manager.Probe(context.Background())
	if err != nil || requests != 1 || status.State != "ready" {
		t.Fatalf("probe status=%#v requests=%d err=%v", status, requests, err)
	}
	cached, err := manager.Status(context.Background())
	if err != nil || cached.State != "ready" || requests != 1 {
		t.Fatalf("cached status=%#v requests=%d err=%v", cached, requests, err)
	}
}

func TestManagerProbeRejectsMissingPiAddress(t *testing.T) {
	manager := &Manager{statusErr: os.ErrNotExist}
	if _, err := manager.Probe(context.Background()); err == nil {
		t.Fatal("missing Pi client passed a fresh status probe")
	}
}

func TestManagementURLUsesFixedPortAndIPv6Brackets(t *testing.T) {
	if got := managementURL("2001:db8::10"); got != "https://[2001:db8::10]:9443" {
		t.Fatalf("IPv6 management URL = %q", got)
	}
}
