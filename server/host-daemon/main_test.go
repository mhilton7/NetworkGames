package main

import (
	"context"
	"crypto/sha256"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	webauth "wiibridge/server/host-daemon/auth"
	"wiibridge/server/host-daemon/bridgecontrol"
	"wiibridge/server/host-daemon/exportprofile"
	"wiibridge/server/host-daemon/gamecube"
	"wiibridge/server/host-daemon/scanner"
	"wiibridge/server/host-daemon/vdisk"
	webui "wiibridge/server/host-daemon/web"
	"wiibridge/shared/model"
	"wiibridge/tests/testutil"
)

type fakePiController struct {
	actions   []string
	probes    int
	fail      string
	status    bridgecontrol.Status
	statusErr error
	address   string
}

func TestStartupHandlerExposesProgressBeforeLibraryIsReady(t *testing.T) {
	startup := &startupHandler{}
	startup.SetPhase("Scanning GameCube library")

	health := httptest.NewRecorder()
	startup.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK ||
		!strings.Contains(health.Body.String(), `"status":"starting"`) ||
		!strings.Contains(health.Body.String(), "Scanning GameCube library") {
		t.Fatalf("startup health status=%d body=%s", health.Code, health.Body.String())
	}
	ready := httptest.NewRecorder()
	startup.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable ||
		!strings.Contains(ready.Body.String(), `"status":"not-ready"`) {
		t.Fatalf("startup readiness status=%d body=%s", ready.Code, ready.Body.String())
	}

	page := httptest.NewRecorder()
	startup.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/login", nil))
	if page.Code != http.StatusOK ||
		!strings.Contains(page.Body.String(), "WiiBridge is starting") ||
		!strings.Contains(page.Body.String(), "refreshes automatically") {
		t.Fatalf("startup page status=%d body=%s", page.Code, page.Body.String())
	}
}

func TestStartupLivenessDoesNotWaitForLongLibraryPhase(t *testing.T) {
	startup := &startupHandler{}
	startup.SetPhase("Checking existing GameCube generation")
	release := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		<-release // Simulates a slow scanner or validator after HTTPS is live.
	}()
	t.Cleanup(func() {
		close(release)
		<-finished
	})
	server := httptest.NewServer(startup)
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	started := time.Now()
	response, err := client.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || time.Since(started) >= time.Second {
		t.Fatalf("liveness waited for long phase: status=%d elapsed=%s",
			response.StatusCode, time.Since(started))
	}
}

func TestStartupHandlerKeepsLivenessAvailableAfterFailure(t *testing.T) {
	startup := &startupHandler{}
	startup.SetPhase("Scanning Wii library")
	startup.Fail("Wii library scan failed")

	health := httptest.NewRecorder()
	startup.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK ||
		!strings.Contains(health.Body.String(), `"status":"failed"`) ||
		!strings.Contains(health.Body.String(), `"phase":"Startup failed"`) {
		t.Fatalf("failure health status=%d body=%s", health.Code, health.Body.String())
	}
	page := httptest.NewRecorder()
	startup.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK ||
		!strings.Contains(page.Body.String(), "WiiBridge startup failed") ||
		!strings.Contains(page.Body.String(), "Wii library scan failed") {
		t.Fatalf("failure page status=%d body=%s", page.Code, page.Body.String())
	}
}

func TestWiiReadinessDoesNotWaitForGameCubeValidation(t *testing.T) {
	a := testApp(t)
	a.mu.Lock()
	a.ready = true
	a.gcStartupPhase = "Validating GameCube library"
	a.mu.Unlock()
	response := httptest.NewRecorder()
	a.readyHealth(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"status":"ready"`) {
		t.Fatalf("Wii readiness blocked by GameCube: status=%d body=%s",
			response.Code, response.Body.String())
	}
}

func TestReadinessRejectsIncompleteWiiExport(t *testing.T) {
	a := testApp(t)
	a.mu.Lock()
	a.ready = false
	a.exports = nil
	a.mu.Unlock()
	response := httptest.NewRecorder()
	a.readyHealth(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable ||
		!strings.Contains(response.Body.String(), `"status":"not-ready"`) {
		t.Fatalf("incomplete Wii export was ready: status=%d body=%s",
			response.Code, response.Body.String())
	}
}

func TestVersionReportsBuildIdentity(t *testing.T) {
	oldCommit, oldTime, oldDirty := gitCommit, buildTime, buildDirty
	t.Cleanup(func() {
		gitCommit, buildTime, buildDirty = oldCommit, oldTime, oldDirty
	})
	gitCommit = "0123456789abcdef"
	buildTime = "2026-07-28T00:00:00Z"
	buildDirty = "false"
	got := buildVersion()
	for _, expected := range []string{
		"WiiBridge " + version,
		"commit 0123456789abcdef",
		"built 2026-07-28T00:00:00Z",
		"dirty false",
		"target " + runtime.GOOS + "/" + runtime.GOARCH,
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("version output %q lacks %q", got, expected)
		}
	}
}

func TestHealthCheckAcceptsTrustedLoopbackCertificateWithoutIPSAN(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, map[string]string{"status": "starting"})
	}))
	defer server.Close()

	caPath := filepath.Join(t.TempDir(), "ca.crt")
	certificate := server.Certificate()
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: certificate.Raw,
	}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runHealthCheck([]string{
		"--url", server.URL + "/healthz", "--ca", caPath,
	}); err != nil {
		t.Fatalf("trusted loopback health check failed: %v", err)
	}
}

func TestHealthCheckReportsCAAndHTTPFailures(t *testing.T) {
	if err := runHealthCheck([]string{
		"--url", "https://127.0.0.1:1/healthz",
		"--ca", filepath.Join(t.TempDir(), "missing.crt"),
	}); err == nil || !strings.Contains(err.Error(), "read CA") {
		t.Fatalf("missing CA error=%v", err)
	}
}

func TestSwitchingHandlerAtomicallyPublishesReadyUI(t *testing.T) {
	startup := &startupHandler{}
	startup.SetPhase("Scanning Wii library")
	handler := &switchingHandler{handler: startup}

	before := httptest.NewRecorder()
	handler.ServeHTTP(before, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(before.Body.String(), "WiiBridge is starting") {
		t.Fatalf("startup handler was not active: %s", before.Body.String())
	}

	handler.Set(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ready"))
	}))
	after := httptest.NewRecorder()
	handler.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/", nil))
	if after.Body.String() != "ready" {
		t.Fatalf("ready handler was not published: %s", after.Body.String())
	}
}

func (f *fakePiController) Action(_ context.Context, action string) error {
	f.actions = append(f.actions, action)
	if action == f.fail {
		return errors.New("synthetic Pi failure")
	}
	return nil
}

func (f *fakePiController) Probe(_ context.Context) (bridgecontrol.Status, error) {
	f.probes++
	return f.status, f.statusErr
}

func (f *fakePiController) Status(_ context.Context) (bridgecontrol.Status, error) {
	return f.status, f.statusErr
}

func (f *fakePiController) Address() string { return f.address }

func (f *fakePiController) SetAddress(_ context.Context, address string) error {
	if f.fail == "address" {
		return errors.New("synthetic address failure")
	}
	f.address = address
	return nil
}

func readyPiController() *fakePiController {
	return &fakePiController{status: bridgecontrol.Status{
		Target: "zero-w-armhf", Board: "Raspberry Pi Zero W Rev 1.1",
		BoardOK: true, Provisioned: true, WiFiReady: true,
		USBController: "20980000.usb", USBState: "not attached", State: "ready",
	}}
}

func testApp(t *testing.T) *app {
	t.Helper()
	disk, err := vdisk.Build("all", nil, version)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	gcConfig := gamecube.DefaultLibraryConfig()
	gcConfig.SourceRoot = dataDir
	gcLibrary, err := gamecube.NewLibraryManager(
		dataDir+"/gamecube/library", gcConfig)
	if err != nil {
		t.Fatal(err)
	}
	browser, err := webauth.New(dataDir+"/auth", "admin", "wiibridge", 12*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := webui.New(webui.Functions(humanBytes))
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
		dataDir: dataDir, gcLibrary: gcLibrary, gcMode: gamecube.MemoryCardPhysical,
		browser: browser, web: renderer, failures: make(map[string]authFailure),
	}
	a.wii = &wiiExportProfile{app: a}
	a.exports, err = exportprofile.New(a.wii)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDisabledPiManagerProducesNilController(t *testing.T) {
	controller := configuredPiController(nil)
	if controller != nil {
		t.Fatal("disabled Pi manager became a non-nil controller interface")
	}
}

func TestBrowserAuthFormsExposePasswordManagerMetadata(t *testing.T) {
	a := testApp(t)
	login := httptest.NewRecorder()
	a.loginForm(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	loginBody := login.Body.String()
	for _, expected := range []string{
		`action="/login" class="form-stack" autocomplete="on"`,
		`id="login-username" type="text" name="username" autocomplete="username"`,
		`id="login-password" type="password" name="password" autocomplete="current-password"`,
	} {
		if !strings.Contains(loginBody, expected) {
			t.Errorf("login form missing password-manager field %q", expected)
		}
	}

	var password strings.Builder
	if err := a.web.Execute(&password, "change-password.html",
		map[string]any{"CSRF": "test-csrf"}); err != nil {
		t.Fatal(err)
	}
	passwordBody := password.String()
	for _, expected := range []string{
		`action="/account/password" class="form-stack" autocomplete="on"`,
		`id="current-password" type="password" name="current" autocomplete="current-password"`,
		`id="new-password" type="password" name="password" autocomplete="new-password"`,
		`id="confirm-password" type="password" name="confirm" autocomplete="new-password"`,
	} {
		if !strings.Contains(passwordBody, expected) {
			t.Errorf("password-change form missing password-manager field %q", expected)
		}
	}
	if strings.Contains(loginBody, `autocomplete="off"`) ||
		strings.Contains(passwordBody, `autocomplete="off"`) {
		t.Fatal("browser credential forms must not disable password managers")
	}
}

func TestDashboardKeepsPiIndicatorsVisibleWhenUnconfigured(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest("GET", "/", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	for _, expected := range []string{
		"Raspberry Pi Bridge", "Raspberry Pi address",
		"Pi status", "Wii connection", "/assets/app.js",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("unconfigured dashboard missing %q", expected)
		}
	}
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
		"Complete Wii Catalog", "Complete GameCube Catalog", "Catalog Viewer",
		"Files Needing Attention", "broken.wbfs", "invalid WBFS magic",
		"unsupported.nkit.iso", "NKit images are unsupported", "Scan again",
		"Activate Wii Library", "Activate GameCube Library",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("dashboard missing %q", expected)
		}
	}
}

func TestDashboardGroupsPiPowerWithBridgeAndCollapsesCatalogViewer(t *testing.T) {
	a := testApp(t)
	a.pi = readyPiController()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	profiles := strings.Index(body, `aria-label="Complete library profiles"`)
	piPanel := strings.Index(body, `id="pi-live"`)
	power := strings.Index(body, "Power options")
	viewer := strings.Index(body, `<details class="card catalog-viewer"`)
	if profiles < 0 || piPanel < 0 || power < 0 || viewer < 0 ||
		!(profiles < piPanel && piPanel < power && power < viewer) {
		t.Fatalf("dashboard order profiles=%d pi=%d power=%d viewer=%d",
			profiles, piPanel, power, viewer)
	}
	for _, expected := range []string{
		"<summary><span><strong>Catalog Viewer</strong>",
		`data-persist-details="catalog-viewer"`,
		`data-persist-details="source-review"`,
		`class="confirmed-action"`,
		"Connection controls", "Reboot Raspberry Pi", "Shut Down Raspberry Pi",
		"Complete Wii Catalog", "Complete GameCube Catalog",
		"Activate Wii Library", "Activate GameCube Library",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("dashboard missing %q", expected)
		}
	}
	if strings.Contains(body, `class="card power`) {
		t.Error("power controls must not be rendered outside the Pi bridge panel")
	}
}

func TestLibraryReviewSeparatesScannerCounts(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest("GET", "/", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	for _, expected := range []string{
		"Files Needing Attention", "2 items", "broken.wbfs", "unsupported.nkit.iso",
	} {
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
	pi := readyPiController()
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
	pi := readyPiController()
	pi.fail = "attach"
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

func TestAutomaticSwitchRejectsUnavailablePiBeforeChangingExport(t *testing.T) {
	a := testApp(t)
	pi := &fakePiController{statusErr: errors.New("synthetic unavailable Pi")}
	a.pi = pi
	previousPlatform := a.exports.Platform()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/export/wii", nil)
	request.SetPathValue("platform", "wii")
	response := httptest.NewRecorder()
	a.selectExport(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if pi.probes != 1 || len(pi.actions) != 0 ||
		a.exports.Platform() != previousPlatform {
		t.Fatalf("unavailable Pi changed state: probes=%d actions=%v platform=%s",
			pi.probes, pi.actions, a.exports.Platform())
	}
}

func TestAutomaticSwitchRejectsUnreadyPiBeforeChangingExport(t *testing.T) {
	a := testApp(t)
	pi := readyPiController()
	pi.status.State = "setup"
	pi.status.Provisioned = false
	a.pi = pi
	previousPlatform := a.exports.Platform()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/export/wii", nil)
	request.SetPathValue("platform", "wii")
	response := httptest.NewRecorder()
	a.selectExport(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if pi.probes != 1 || len(pi.actions) != 0 ||
		a.exports.Platform() != previousPlatform {
		t.Fatalf("unready Pi changed state: probes=%d actions=%v platform=%s",
			pi.probes, pi.actions, a.exports.Platform())
	}
}

func TestPiPowerControlsRequireConfirmation(t *testing.T) {
	a := testApp(t)
	pi := &fakePiController{}
	a.pi = pi
	request := httptest.NewRequest("POST", "/api/v1/pi/reboot", nil)
	request.SetPathValue("action", "reboot")
	response := httptest.NewRecorder()
	a.piPowerAction(response, request)
	if response.Code != 400 || len(pi.actions) != 0 {
		t.Fatalf("unconfirmed reboot was not rejected: status=%d actions=%v",
			response.Code, pi.actions)
	}
}

func TestConfirmedShutdownUsesFixedPoweroffAction(t *testing.T) {
	a := testApp(t)
	pi := &fakePiController{}
	a.pi = pi
	request := httptest.NewRequest("POST", "/api/v1/pi/shutdown?confirm=shutdown", nil)
	request.SetPathValue("action", "shutdown")
	response := httptest.NewRecorder()
	a.piPowerAction(response, request)
	if response.Code != 200 || strings.Join(pi.actions, ",") != "poweroff" {
		t.Fatalf("shutdown status=%d actions=%v body=%s",
			response.Code, pi.actions, response.Body.String())
	}
}

func TestPiPowerControlsAreUnavailableWithoutCoordinator(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest("POST", "/api/v1/pi/reboot?confirm=reboot", nil)
	request.SetPathValue("action", "reboot")
	response := httptest.NewRecorder()
	a.piPowerAction(response, request)
	if response.Code != 503 {
		t.Fatalf("unconfigured Pi control status=%d, want 503", response.Code)
	}
}

func TestPiStatusReturnsLiveOperationalState(t *testing.T) {
	a := testApp(t)
	a.pi = &fakePiController{status: bridgecontrol.Status{
		Target: "zero-w-armhf", Board: "Raspberry Pi Zero W Rev 1.1",
		BoardOK: true, Provisioned: true, WiFiReady: true, AutoAttach: true,
		NBDConnected: true, USBAttached: true, ExportMode: "wii",
		USBController: "20980000.usb", USBState: "configured",
		Addresses: []string{"192.0.2.10/24"}, State: "ready",
	}}
	request := httptest.NewRequest("GET", "/api/v1/pi/status", nil)
	response := httptest.NewRecorder()
	a.piStatus(response, request)
	if response.Code != 200 {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, expected := range []string{
		`"connected":true`, `"export_mode":"wii"`,
		`"nbd_connected":true`, `"usb_attached":true`,
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("live status missing %q", expected)
		}
	}
}

func TestPiStatusFailureDoesNotLeakConnectionDetails(t *testing.T) {
	a := testApp(t)
	a.pi = &fakePiController{statusErr: errors.New("dial 192.0.2.1: secret failure")}
	request := httptest.NewRequest("GET", "/api/v1/pi/status", nil)
	response := httptest.NewRecorder()
	a.piStatus(response, request)
	if response.Code != 502 || strings.Contains(response.Body.String(), "192.0.2.1") {
		t.Fatalf("unsafe failure response: status=%d body=%s",
			response.Code, response.Body.String())
	}
}

func TestDashboardShowsLivePiStatusPanel(t *testing.T) {
	a := testApp(t)
	a.pi = &fakePiController{status: bridgecontrol.Status{
		Board: "Raspberry Pi Zero W Rev 1.1", Provisioned: true,
		AutoAttach: true, NBDConnected: true, USBAttached: true,
		ExportMode: "wii", USBController: "20980000.usb",
		USBState: "configured", State: "ready",
	}, address: "192.0.2.10"}
	request := httptest.NewRequest("GET", "/", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	for _, expected := range []string{
		"Raspberry Pi Bridge", `value="192.0.2.10"`,
		"/assets/app.js", "Reconcile Connection",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("dashboard missing %q", expected)
		}
	}
}

func TestPiAddressControlUsesConfiguredManager(t *testing.T) {
	a := testApp(t)
	pi := &fakePiController{address: "192.0.2.10"}
	a.pi = pi
	request := httptest.NewRequest("POST", "/api/v1/pi/address?address=192.0.2.20", nil)
	response := httptest.NewRecorder()
	a.setPiAddress(response, request)
	if response.Code != 200 || pi.address != "192.0.2.20" {
		t.Fatalf("address update status=%d address=%q body=%s",
			response.Code, pi.address, response.Body.String())
	}
}

func TestBrowserRedirectAPITokenCompatibilityAndBootstrapRestriction(t *testing.T) {
	a := testApp(t)
	token := "this-is-a-valid-api-token"
	a.tokenSum = sha256.Sum256([]byte(token))
	protected := a.auth(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	browserRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	browserRequest.Header.Set("Accept", "text/html")
	browserResponse := httptest.NewRecorder()
	protected(browserResponse, browserRequest)
	if browserResponse.Code != http.StatusSeeOther ||
		browserResponse.Header().Get("Location") != "/login" {
		t.Fatalf("browser auth response=%d location=%q",
			browserResponse.Code, browserResponse.Header().Get("Location"))
	}
	apiRequest := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	apiResponse := httptest.NewRecorder()
	protected(apiResponse, apiRequest)
	if apiResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated API status=%d", apiResponse.Code)
	}
	for _, authorize := range []func(*http.Request){
		func(request *http.Request) { request.Header.Set("Authorization", "Bearer "+token) },
		func(request *http.Request) { request.SetBasicAuth("admin", token) },
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
		authorize(request)
		response := httptest.NewRecorder()
		protected(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("compatible API authentication status=%d", response.Code)
		}
	}
	form := url.Values{
		"csrf": {a.csrf}, "username": {"admin"}, "password": {"wiibridge"},
	}
	loginRequest := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(form.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResponse := httptest.NewRecorder()
	a.login(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusSeeOther ||
		loginResponse.Header().Get("Location") != "/account/password" {
		t.Fatalf("bootstrap login status=%d location=%q",
			loginResponse.Code, loginResponse.Header().Get("Location"))
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly ||
		cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe session cookie: %#v", cookies)
	}
	session, ok := a.browser.Validate(cookies[0].Value)
	if !ok || !session.PasswordChange {
		t.Fatal("bootstrap session is not password-change restricted")
	}
	mutation := httptest.NewRequest(http.MethodPost, "/api/v1/export/wii",
		strings.NewReader(url.Values{"csrf": {session.CSRF}}.Encode()))
	mutation.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mutation.AddCookie(cookies[0])
	mutationResponse := httptest.NewRecorder()
	protected(mutationResponse, mutation)
	if mutationResponse.Code != http.StatusForbidden {
		t.Fatalf("bootstrap session mutation status=%d", mutationResponse.Code)
	}
}

func TestDashboardDisablesAutomaticSwitchingWhilePiIsUnavailable(t *testing.T) {
	a := testApp(t)
	a.pi = &fakePiController{
		statusErr: errors.New("synthetic unavailable Pi"),
		address:   "192.0.2.10",
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK ||
		!strings.Contains(body, "Connect it before switching libraries") ||
		strings.Count(body, `data-pi-guard="blocked"`) != 2 {
		t.Fatalf("unavailable Pi did not disable profile switches: status=%d body=%s",
			response.Code, body)
	}
}

func TestDashboardExactLibraryControlsAndNoPerTitleActivation(t *testing.T) {
	a := testApp(t)
	a.pi = &fakePiController{}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	for _, required := range []string{
		"Activate Wii Library", "Activate GameCube Library",
		"Build GameCube Library", "GameCube Library", "Wii Library",
		"Safely Detach USB", "Disconnect NBD", "Connect Current Library",
		"Attach USB", "Reconcile Connection", "Reboot Raspberry Pi",
		"Shut Down Raspberry Pi", "Log out",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("dashboard missing %q", required)
		}
	}
	if strings.Contains(body, "Prepare export") ||
		strings.Contains(body, `name="id"`) {
		t.Fatal("primary dashboard exposes per-title activation")
	}
	if strings.Contains(body, "https://") || strings.Contains(body, "http://") {
		t.Fatal("dashboard contains an external asset URL")
	}
}

func TestCompleteGameCubeActivationNeedsNoGameIDAndUsesSafeSequence(t *testing.T) {
	a := testApp(t)
	sourceRoot := filepath.Join(a.dataDir, "gc-source")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceRoot, "game.iso")
	if err := testutil.SyntheticGameCubeISO(
		source, "GACT01", "Activation Test", 0, 0, 2<<20); err != nil {
		t.Fatal(err)
	}
	scan, err := gamecube.Scan(sourceRoot)
	if err != nil || len(scan.Games) != 1 {
		t.Fatalf("scan=%#v err=%v", scan, err)
	}
	if _, err = a.gcLibrary.Build(context.Background(), scan.Games); err != nil {
		t.Fatal(err)
	}
	pi := readyPiController()
	a.pi = pi
	request := httptest.NewRequest(http.MethodPost, "/api/v1/export/gamecube", nil)
	request.SetPathValue("platform", "gamecube")
	response := httptest.NewRecorder()
	a.selectExport(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("activation status=%d body=%s", response.Code, response.Body.String())
	}
	if got := strings.Join(pi.actions, ","); got !=
		"detach,disconnect,connect-gamecube-physical,attach" {
		t.Fatalf("unsafe aggregate activation sequence: %s", got)
	}
	backend, release, err := a.exports.BeginSession()
	if err != nil {
		t.Fatal(err)
	}
	mode, ok := backend.(interface{ ReadOnly() bool })
	release()
	if a.exports.Platform() != "gamecube" || !ok || !mode.ReadOnly() {
		t.Fatalf("aggregate profile platform=%s mode=%T",
			a.exports.Platform(), backend)
	}
}

func TestPiStorageControlsDeriveActionsAndRejectRawNames(t *testing.T) {
	a := testApp(t)
	pi := &fakePiController{}
	a.pi = pi
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pi/storage/disconnect", nil)
	request.SetPathValue("action", "disconnect")
	response := httptest.NewRecorder()
	a.piStorageAction(response, request)
	if got := strings.Join(pi.actions, ","); got != "detach,disconnect" {
		t.Fatalf("disconnect sequence=%s", got)
	}
	pi.actions = nil
	request = httptest.NewRequest(http.MethodPost, "/api/v1/pi/storage/systemctl", nil)
	request.SetPathValue("action", "systemctl")
	response = httptest.NewRecorder()
	a.piStorageAction(response, request)
	if response.Code != http.StatusBadRequest || len(pi.actions) != 0 {
		t.Fatalf("raw action accepted: status=%d actions=%v", response.Code, pi.actions)
	}
}

func TestPiAddressControlRejectsURLs(t *testing.T) {
	a := testApp(t)
	pi := &fakePiController{address: "192.0.2.10"}
	a.pi = pi
	request := httptest.NewRequest(
		"POST", "/api/v1/pi/address?address=https://attacker.invalid", nil)
	response := httptest.NewRecorder()
	a.setPiAddress(response, request)
	if response.Code != 400 || pi.address != "192.0.2.10" {
		t.Fatalf("unsafe address status=%d address=%q", response.Code, pi.address)
	}
}
