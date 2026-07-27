package main

import (
	"context"
	"errors"
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

type fakePiController struct {
	actions []string
	fail    string
}

func (f *fakePiController) Action(_ context.Context, action string) error {
	f.actions = append(f.actions, action)
	if action == f.fail {
		return errors.New("synthetic Pi failure")
	}
	return nil
}

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
		}}, Rejected: []scanner.Rejection{{
			Path: "/library/broken.wbfs", Reason: "invalid WBFS magic",
		}}},
		gcScan: gamecube.Result{Games: []gamecube.Game{{
			ID: "GTEST0", Title: "Synthetic GameCube", Region: "USA",
			Format: "iso", DiscCount: 1, Validation: "valid",
		}}, Rejected: []gamecube.Rejection{{
			Path: "/library/unsupported.nkit.iso", Reason: "NKit images are unsupported",
		}}},
		root:    "/library",
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
		"Wii remains the safe default export", "Published snapshot", "Library summary",
		"Library review", "broken.wbfs", "invalid WBFS magic",
		"unsupported.nkit.iso", "NKit images are unsupported", "Rescan library",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("dashboard missing %q", expected)
		}
	}
}

func TestLibraryReviewSeparatesScannerCounts(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest("GET", "/", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	for _, expected := range []string{"1 Wii", "1 GameCube", "2 items", "open details"} {
		if !strings.Contains(body, expected) {
			t.Errorf("review panel missing %q", expected)
		}
	}
}

func TestDashboardSearchFiltersBothPlatforms(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest("GET", "/?platform=all&q=gamecube", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	if strings.Contains(body, "Synthetic Wii") {
		t.Fatal("Wii result did not respect the search query")
	}
	if !strings.Contains(body, "Synthetic GameCube") || !strings.Contains(body, `value="gamecube"`) {
		t.Fatal("GameCube search result or retained query is missing")
	}
}

func TestHumanBytes(t *testing.T) {
	tests := map[int64]string{512: "512 B", 1024: "1.0 KiB", 5 << 20: "5.0 MiB"}
	for size, expected := range tests {
		if got := humanBytes(size); got != expected {
			t.Errorf("humanBytes(%d) = %q, want %q", size, got, expected)
		}
	}
}

func TestBrowserActionsRedirectBackToDashboard(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/v1/export/wii", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response := httptest.NewRecorder()
	respondAction(response, request, 200, map[string]string{"status": "ok"},
		"Export switched.", "wii")
	if response.Code != 303 {
		t.Fatalf("status = %d, want 303", response.Code)
	}
	location := response.Header().Get("Location")
	if !strings.Contains(location, "notice=Export+switched") ||
		!strings.Contains(location, "platform=wii") {
		t.Fatalf("unexpected redirect %q", location)
	}
}

func TestAutomaticWiiSwitchUsesSafeActionOrder(t *testing.T) {
	a := testApp(t)
	pi := &fakePiController{}
	a.pi = pi
	request := httptest.NewRequest("POST", "/api/v1/export/wii", nil)
	request.SetPathValue("platform", "wii")
	response := httptest.NewRecorder()
	a.selectExport(response, request)
	if response.Code != 200 {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	got := strings.Join(pi.actions, ",")
	if got != "detach,disconnect,connect-wii,attach" {
		t.Fatalf("unsafe automatic action order: %s", got)
	}
}

func TestAutomaticAttachFailureLeavesPiDisconnected(t *testing.T) {
	a := testApp(t)
	pi := &fakePiController{fail: "attach"}
	a.pi = pi
	request := httptest.NewRequest("POST", "/api/v1/export/wii", nil)
	request.SetPathValue("platform", "wii")
	response := httptest.NewRecorder()
	a.selectExport(response, request)
	if response.Code != 502 {
		t.Fatalf("status = %d, want 502", response.Code)
	}
	got := strings.Join(pi.actions, ",")
	if got != "detach,disconnect,connect-wii,attach,detach,disconnect" {
		t.Fatalf("failed switch did not return to safe detached state: %s", got)
	}
}
