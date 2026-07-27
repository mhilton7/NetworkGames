package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wiibridge/server/host-daemon/exportprofile"
	"wiibridge/server/host-daemon/gamecube"
	"wiibridge/server/host-daemon/scanner"
	"wiibridge/server/host-daemon/vdisk"
	"wiibridge/shared/model"
)

func testApp(t *testing.T) *app {
	t.Helper()
	disk, err := vdisk.Build("all", nil, version)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{
		disk: disk,
		scan: scanner.Result{Games: []model.Game{{
			ID: "TEST01", Title: "Synthetic Wii", Size: 512,
		}}},
		gcScan: gamecube.Result{Games: []gamecube.Game{{
			ID: "GTEST0", Title: "Synthetic GameCube", Region: "USA",
			Format: "iso", DiscCount: 1, Validation: "valid",
		}}},
		started: time.Now(),
		csrf:    "test-csrf",
	}
	a.wii = &wiiExportProfile{app: a}
	a.exports, err = exportprofile.New(a.wii)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestWiiProfileRemainsDefaultAndUsesCurrentSnapshot(t *testing.T) {
	a := testApp(t)
	if got := a.exports.Platform(); got != "wii" {
		t.Fatalf("default platform = %q, want wii", got)
	}
	first := a.wii.Backend()
	replacement, err := vdisk.Build("rescanned", nil, version)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.disk = replacement
	a.mu.Unlock()
	if got := a.wii.Backend(); got == first || got != replacement {
		t.Fatal("Wii profile did not retain rescan behavior")
	}
	if !a.wii.ReadOnly() {
		t.Fatal("Wii profile must remain read-only")
	}
}

func TestDashboardProvidesPlatformFiltersWithoutHidingWii(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest("GET", "/?platform=all", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	for _, expected := range []string{
		"All", "Wii", "GameCube", "Synthetic Wii", "Synthetic GameCube",
		"Wii remains the default export",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("dashboard missing %q", expected)
		}
	}
}
