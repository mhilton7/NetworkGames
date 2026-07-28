package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validWiFiProvision() provisionRequest {
	return provisionRequest{
		WiFiCountry: "US",
		WiFiSSID:    "Test Network",
		WiFiPSK:     "correct-horse",
	}
}

func TestValidateProvisionWiFiOnly(t *testing.T) {
	if err := validateProvision(validWiFiProvision()); err != nil {
		t.Fatalf("valid Wi-Fi provision rejected: %v", err)
	}
}

func TestValidateProvisionAllowsStoredWiFiToRemainUnchanged(t *testing.T) {
	if err := validateProvision(provisionRequest{}); err != nil {
		t.Fatalf("unchanged Wi-Fi provision rejected: %v", err)
	}
}

func TestValidateProvisionRejectsPartialWiFiUpdate(t *testing.T) {
	tests := []provisionRequest{
		{WiFiCountry: "US"},
		{WiFiSSID: "Test Network"},
		{WiFiPSK: "correct-horse"},
	}
	for _, request := range tests {
		if err := validateProvision(request); err == nil {
			t.Fatal("partial Wi-Fi update was accepted")
		}
	}
}

func TestValidateProvisionRejectsUnsafeValues(t *testing.T) {
	tests := []struct {
		name   string
		change func(*provisionRequest)
	}{
		{"country", func(p *provisionRequest) { p.WiFiCountry = "USA" }},
		{"ssid newline", func(p *provisionRequest) { p.WiFiSSID = "bad\nssid" }},
		{"short password", func(p *provisionRequest) { p.WiFiPSK = "short" }},
		{"password newline", func(p *provisionRequest) { p.WiFiPSK = "password\n" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := validWiFiProvision()
			test.change(&p)
			if err := validateProvision(p); err == nil {
				t.Fatal("unsafe provision was accepted")
			}
		})
	}
}

func TestValidateProvisionRequiresCompleteBridge(t *testing.T) {
	p := validWiFiProvision()
	p.Bridge = true
	p.NBDHost = "server.example"
	p.NBDPort = "10809"
	p.NBDExport = "catalog"
	if err := validateProvision(p); err == nil ||
		!strings.Contains(err.Error(), "certificate") {
		t.Fatalf("incomplete bridge returned %v", err)
	}
}

func TestValidateProvisionRequiresUSBIdentityForAutoAttach(t *testing.T) {
	p := validWiFiProvision()
	p.Bridge = true
	p.AutoAttach = true
	p.NBDHost = "server.example"
	p.NBDPort = "10809"
	p.NBDExport = "catalog"
	if err := validateProvision(p); err == nil ||
		!strings.Contains(err.Error(), "USB VID and PID") {
		t.Fatalf("automatic attachment without USB identity returned %v", err)
	}
}

func TestProvisionFilesCanRetainWiFi(t *testing.T) {
	files := provisionFiles(provisionRequest{AutoAttach: true})
	if files["wifi-update"] != "0" || files["auto-attach"] != "1" {
		t.Fatalf("unexpected staging flags: %#v", files)
	}
	for _, name := range []string{"wifi-country", "wifi-ssid", "wifi-password"} {
		if _, ok := files[name]; ok {
			t.Fatalf("unchanged Wi-Fi staged %s", name)
		}
	}
}

func TestCSRFTokenIsStableAndScoped(t *testing.T) {
	first := csrfToken("device-token")
	second := csrfToken("device-token")
	if first != second || len(first) != 64 {
		t.Fatalf("unexpected CSRF token %q", first)
	}
	if first == csrfToken("other-device-token") {
		t.Fatal("different admin tokens produced the same CSRF token")
	}
}

func TestReadAdminTokenAcceptsTwelveCharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.token")
	if err := os.WriteFile(path, []byte("a1b2c3d4e5f6\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readAdminToken(path)
	if err != nil {
		t.Fatalf("12-character password rejected: %v", err)
	}
	if got != "a1b2c3d4e5f6" {
		t.Fatalf("unexpected password %q", got)
	}
}

func TestReadAdminTokenRejectsShortPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.token")
	if err := os.WriteFile(path, []byte("too-short\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readAdminToken(path); err == nil {
		t.Fatal("short management password was accepted")
	}
}

func TestAuthLimiterResetsAfterSuccess(t *testing.T) {
	limiter := newAuthLimiter(2, time.Minute)
	limiter.failed("client")
	limiter.failed("client")
	if limiter.allowed("client") {
		t.Fatal("client should be limited")
	}
	limiter.succeeded("client")
	if !limiter.allowed("client") {
		t.Fatal("successful authentication should clear failures")
	}
}

func TestBrowserActionUsesCSRFAndInvokesTypedHelper(t *testing.T) {
	called := ""
	app := &controller{
		token:   "a1b2c3d4e5f6",
		target:  "zero-w-armhf",
		csrf:    csrfToken("a1b2c3d4e5f6"),
		limiter: newAuthLimiter(10, time.Minute),
		runHelper: func(_ context.Context, action string) ([]byte, error) {
			called = action
			return nil, nil
		},
	}
	form := url.Values{"csrf": {app.csrf}}
	request := httptest.NewRequest(http.MethodPost, "/action/test",
		strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth("admin", app.token)
	response := httptest.NewRecorder()

	app.routes().ServeHTTP(response, request)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("browser action returned status %d", response.Code)
	}
	if called != "test" {
		t.Fatalf("typed helper action = %q, want test", called)
	}
}

func TestPowerControlsAreExplicitTypedActions(t *testing.T) {
	for _, action := range []string{"poweroff", "reboot"} {
		if !validAction(action) {
			t.Fatalf("safe %s action was rejected", action)
		}
	}
}

func TestGameCubeConnectionModesAreApprovedButRemainExplicit(t *testing.T) {
	for _, action := range []string{
		"connect-wii", "connect-gamecube-physical", "connect-gamecube-emulated",
	} {
		if !validAction(action) {
			t.Fatalf("%s was rejected", action)
		}
	}
	if validAction("connect-gamecube") {
		t.Fatal("ambiguous GameCube write mode was accepted")
	}
}

func TestUSBResetObservationIgnoresIntentionalDetach(t *testing.T) {
	if !observedUSBReset("configured", "not attached", true) {
		t.Fatal("configured-to-unconfigured transition was not counted")
	}
	if observedUSBReset("configured", "not attached", false) {
		t.Fatal("intentional detach was counted as a reset")
	}
	if observedUSBReset("not attached", "configured", true) {
		t.Fatal("normal attach was counted as a reset")
	}
}

func TestDashboardIncludesSafePowerControls(t *testing.T) {
	var output strings.Builder
	err := dashboardTemplate.Execute(&output, map[string]any{
		"Status": status{},
		"CSRF":   "csrf",
		"Actions": []actionButton{
			{Name: "reboot", Label: "Reboot Pi"},
			{Name: "poweroff", Label: "Safely power off Pi"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := output.String()
	if !strings.Contains(body, `action="/action/poweroff"`) ||
		!strings.Contains(body, `action="/action/reboot"`) ||
		!strings.Contains(body, "Safely power off Pi") ||
		!strings.Contains(body, "Reboot Pi") {
		t.Fatal("safe power controls are missing from dashboard")
	}
}

func TestCompatibilityAndMetricsEndpointsRequireAuthentication(t *testing.T) {
	app := &controller{
		token: "a1b2c3d4e5f6", target: "zero-w-armhf",
		csrf: csrfToken("a1b2c3d4e5f6"), limiter: newAuthLimiter(10, time.Minute),
		runHelper: func(context.Context, string) ([]byte, error) { return nil, nil },
	}
	handler := app.routes()
	for _, path := range []string{"/api/v1/compatibility", "/api/v1/metrics"} {
		unauthorized := httptest.NewRecorder()
		handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, path, nil))
		if unauthorized.Code != http.StatusUnauthorized {
			t.Fatalf("%s unauthenticated status=%d", path, unauthorized.Code)
		}
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.SetBasicAuth("admin", app.token)
		authorized := httptest.NewRecorder()
		handler.ServeHTTP(authorized, request)
		if authorized.Code != http.StatusOK {
			t.Fatalf("%s authenticated status=%d body=%s",
				path, authorized.Code, authorized.Body.String())
		}
	}
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if strings.Contains(health.Body.String(), `"capabilities"`) {
		t.Fatal("unauthenticated health response exposed compatibility descriptor")
	}
}

func TestPiMetricsSnapshotIsCached(t *testing.T) {
	app := &controller{target: "zero-w-armhf"}
	first := app.collectMetrics()
	second := app.collectMetrics()
	if first.UpdatedAt.IsZero() || first.UpdatedAt != second.UpdatedAt ||
		first.CollectionDurationUS != second.CollectionDurationUS {
		t.Fatalf("metrics were not cached: first=%#v second=%#v", first, second)
	}
}

func BenchmarkCachedPiMetrics(b *testing.B) {
	app := &controller{target: "zero-w-armhf"}
	_ = app.collectMetrics()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_ = app.collectMetrics()
	}
}

func BenchmarkPiMetricsEndpoint(b *testing.B) {
	app := &controller{
		token: "a1b2c3d4e5f6", target: "zero-w-armhf",
		csrf: csrfToken("a1b2c3d4e5f6"), limiter: newAuthLimiter(10, time.Minute),
		runHelper: func(context.Context, string) ([]byte, error) { return nil, nil },
	}
	handler := app.routes()
	_ = app.collectMetrics()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	request.SetBasicAuth("admin", app.token)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			b.Fatal(response.Code)
		}
	}
}

func TestConfiguredWiFiFormDoesNotRequireReentry(t *testing.T) {
	var output strings.Builder
	err := setupTemplate.Execute(&output, map[string]any{
		"CSRF":        "csrf",
		"Provisioned": true,
		"WiFiReady":   true,
		"AutoAttach":  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := output.String()
	if !strings.Contains(body, "Leave all three Wi-Fi fields blank") {
		t.Fatal("configured Wi-Fi retention guidance is missing")
	}
	if strings.Contains(body, `name="wifi_ssid" maxlength="32" required`) {
		t.Fatal("configured Wi-Fi SSID is still required")
	}
}
