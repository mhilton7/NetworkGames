package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
	fail      string
	status    bridgecontrol.Status
	statusErr error
	address   string
}

func (f *fakePiController) Action(_ context.Context, action string) error {
	f.actions = append(f.actions, action)
	if action == f.fail {
		return errors.New("synthetic Pi failure")
	}
	return nil
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

func TestDashboardKeepsPiIndicatorsVisibleWhenUnconfigured(t *testing.T) {
	a := testApp(t)
	request := httptest.NewRequest("GET", "/", nil)
	response := httptest.NewRecorder()
	a.dashboard(response, request)
	body := response.Body.String()
	for _, expected := range []string{
		"Raspberry Pi bridge", "Configured Pi IP",
		"Controller", "NBD / USB", "/assets/app.js",
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
		"Complete Wii game catalog", "Complete Wii catalog",
		"Source review", "broken.wbfs", "invalid WBFS magic",
		"unsupported.nkit.iso", "NKit images are unsupported", "Rescan library",
		"Activate Wii Library", "Activate GameCube Library",
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
	for _, expected := range []string{
		"Source review", "2 items", "broken.wbfs", "unsupported.nkit.iso",
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
		"Raspberry Pi bridge", `value="192.0.2.10"`,
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
	pi := &fakePiController{}
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
