// wiibridge-pi-controller exposes local status and delegates the small set
// of privileged state transitions to fixed, validated helpers.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"wiibridge/shared/compat"
	"wiibridge/shared/perf"
)

const (
	adminTokenPath  = "/etc/wiibridge/admin.token"
	boardTargetPath = "/usr/share/wiibridge/board-target"
	provisionRoot   = "/run/wiibridge"
	provisionPath   = provisionRoot + "/provision"
	maxSetupBody    = 96 << 10

	minAdminPasswordLength = 12
)

var (
	productVersion = "development"
	gitCommit      = "unknown"
	buildTime      = "unknown"
	buildDirty     = "unknown"
)

var (
	hostPattern    = regexp.MustCompile(`^[A-Za-z0-9.:-]{1,253}$`)
	exportPattern  = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	ssidPattern    = regexp.MustCompile(`^[A-Za-z0-9 _.-]{1,32}$`)
	countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)
	usbIDPattern   = regexp.MustCompile(`^0x[0-9A-Fa-f]{4}$`)
)

type status struct {
	Target        string            `json:"target"`
	Board         string            `json:"detected_board"`
	BoardOK       bool              `json:"board_compatible"`
	Provisioned   bool              `json:"provisioned"`
	WiFiReady     bool              `json:"wifi_provisioned"`
	AutoAttach    bool              `json:"auto_attach"`
	NBDConnected  bool              `json:"nbd_connected"`
	USBAttached   bool              `json:"usb_attached"`
	ExportMode    string            `json:"export_mode"`
	USBController string            `json:"usb_controller"`
	USBState      string            `json:"usb_state"`
	Addresses     []string          `json:"addresses"`
	State         string            `json:"state"`
	Compatibility compat.Descriptor `json:"compatibility"`
}

type provisionRequest struct {
	WiFiCountry string
	WiFiSSID    string
	WiFiPSK     string
	NBDHost     string
	NBDPort     string
	NBDExport   string
	TLSCA       string
	TLSCert     string
	TLSKey      string
	USBVID      string
	USBPID      string
	Bridge      bool
	AutoAttach  bool
}

type controller struct {
	token        string
	target       string
	csrf         string
	limiter      *authLimiter
	provisionMu  sync.Mutex
	metricsMu    sync.Mutex
	metricsAt    time.Time
	metrics      perf.PiSnapshot
	cpuTotal     uint64
	cpuIdle      uint64
	attachCount  uint64
	reconnects   uint64
	actionErrors uint64
	usbResets    uint64
	lastUSBState string
	nbdSectors   uint64
	nbdSample    time.Time
	runHelper    func(context.Context, string) ([]byte, error)
}

type actionButton struct {
	Name  string
	Label string
}

func main() {
	authToken, err := readAdminToken(adminTokenPath)
	if err != nil {
		log.Fatal("unique admin token has not been provisioned")
	}
	expected, err := os.ReadFile(boardTargetPath)
	if err != nil {
		log.Fatal(err)
	}
	app := &controller{
		token:   authToken,
		target:  strings.TrimSpace(string(expected)),
		csrf:    csrfToken(authToken),
		limiter: newAuthLimiter(10, time.Minute),
		runHelper: func(ctx context.Context, name string) ([]byte, error) {
			switch name {
			case "connect", "connect-wii", "connect-gamecube-physical",
				"connect-gamecube-emulated", "disconnect", "attach", "detach",
				"clear-cache", "test", "poweroff", "reboot":
				return exec.CommandContext(ctx, "/usr/bin/sudo", "-n",
					"/usr/libexec/wiibridge-helper", name).CombinedOutput()
			case "provision":
				return exec.CommandContext(ctx, "/usr/bin/sudo", "-n",
					"/usr/libexec/wiibridge-provision").CombinedOutput()
			default:
				return nil, errors.New("unsupported helper")
			}
		},
	}

	server := &http.Server{
		Addr:              ":9443",
		Handler:           headers(app.routes()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(server.ListenAndServeTLS(
		"/etc/wiibridge/device.crt",
		"/etc/wiibridge/device.key",
	))
}

func readAdminToken(path string) (string, error) {
	token, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	password := strings.TrimSpace(string(token))
	if len(password) < minAdminPasswordLength {
		return "", errors.New("admin password is too short")
	}
	return password, nil
}

func (c *controller) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		current := collect(c.target)
		writeJSON(w, map[string]any{
			"target": current.Target, "detected_board": current.Board,
			"board_compatible": current.BoardOK, "provisioned": current.Provisioned,
			"wifi_provisioned": current.WiFiReady, "auto_attach": current.AutoAttach,
			"nbd_connected": current.NBDConnected, "usb_attached": current.USBAttached,
			"export_mode": current.ExportMode, "usb_controller": current.USBController,
			"usb_state": current.USBState, "addresses": current.Addresses,
			"state": current.State,
		})
	})
	mux.HandleFunc("GET /api/v1/status", c.authenticated(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, collect(c.target))
	}))
	mux.HandleFunc("GET /api/v1/compatibility", c.authenticated(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, collect(c.target).Compatibility)
	}))
	mux.HandleFunc("GET /api/v1/metrics", c.authenticated(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, c.collectMetrics())
	}))
	mux.HandleFunc("GET /", c.authenticated(c.dashboard))
	mux.HandleFunc("GET /setup", c.authenticated(c.setupForm))
	mux.HandleFunc("POST /setup", c.authenticated(c.saveSetup))
	mux.HandleFunc("POST /action/{action}", c.authenticated(c.webAction))
	mux.HandleFunc("POST /api/v1/action/{action}", c.authenticated(func(w http.ResponseWriter, r *http.Request) {
		if !validCSRF(r, c.csrf) {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}
		action := r.PathValue("action")
		if !validAction(action) {
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		output, err := c.runHelper(ctx, action)
		c.recordAction(action, err)
		if err != nil {
			http.Error(w, "action failed: "+strings.TrimSpace(string(output)), http.StatusConflict)
			return
		}
		writeJSON(w, collect(c.target))
	}))
	return mux
}

func validAction(action string) bool {
	switch action {
	case "connect", "connect-wii", "connect-gamecube-physical",
		"connect-gamecube-emulated", "disconnect", "attach", "detach",
		"clear-cache", "test", "poweroff", "reboot":
		return true
	default:
		return false
	}
}

func (c *controller) webAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid action request", http.StatusBadRequest)
		return
	}
	if !csrfMatches(r.Form.Get("csrf"), c.csrf) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	action := r.PathValue("action")
	if !validAction(action) {
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	output, err := c.runHelper(ctx, action)
	c.recordAction(action, err)
	if err != nil {
		http.Error(w, "action failed: "+strings.TrimSpace(string(output)), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (c *controller) dashboard(w http.ResponseWriter, _ *http.Request) {
	s := collect(c.target)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTemplate.Execute(w, map[string]any{
		"Status": s,
		"CSRF":   c.csrf,
		"Actions": []actionButton{
			{Name: "test", Label: "Test server"},
			{Name: "connect-wii", Label: "Connect Wii mode (read-only)"},
			{Name: "connect-gamecube-physical", Label: "Connect GameCube (physical card, read-only)"},
			{Name: "connect-gamecube-emulated", Label: "Connect GameCube (emulated card, saves writable)"},
			{Name: "attach", Label: "Attach USB"},
			{Name: "detach", Label: "Detach USB"},
			{Name: "disconnect", Label: "Disconnect server"},
			{Name: "reboot", Label: "Reboot Pi"},
			{Name: "poweroff", Label: "Safely power off Pi"},
		},
	}); err != nil {
		log.Printf("dashboard template: %v", err)
	}
}

func (c *controller) setupForm(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := setupTemplate.Execute(w, map[string]any{
		"CSRF":        c.csrf,
		"Provisioned": exists("/etc/wiibridge/provisioned"),
		"WiFiReady":   exists("/etc/wiibridge/wifi-provisioned"),
		"AutoAttach":  exists("/etc/wiibridge/auto-attach"),
	}); err != nil {
		log.Printf("setup template: %v", err)
	}
}

func (c *controller) saveSetup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxSetupBody)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid or oversized setup request", http.StatusBadRequest)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Form.Get("csrf")), []byte(c.csrf)) != 1 {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	p := provisionRequest{
		WiFiCountry: strings.ToUpper(strings.TrimSpace(r.Form.Get("wifi_country"))),
		WiFiSSID:    strings.TrimSpace(r.Form.Get("wifi_ssid")),
		WiFiPSK:     r.Form.Get("wifi_password"),
		NBDHost:     strings.TrimSpace(r.Form.Get("nbd_host")),
		NBDPort:     strings.TrimSpace(r.Form.Get("nbd_port")),
		NBDExport:   strings.TrimSpace(r.Form.Get("nbd_export")),
		TLSCA:       strings.TrimSpace(r.Form.Get("tls_ca")),
		TLSCert:     strings.TrimSpace(r.Form.Get("tls_cert")),
		TLSKey:      strings.TrimSpace(r.Form.Get("tls_key")),
		USBVID:      strings.TrimSpace(r.Form.Get("usb_vid")),
		USBPID:      strings.TrimSpace(r.Form.Get("usb_pid")),
		AutoAttach:  r.Form.Get("auto_attach") == "1",
	}
	p.Bridge = p.NBDHost != "" || p.NBDExport != "" || p.TLSCA != "" ||
		p.TLSCert != "" || p.TLSKey != "" || p.USBVID != "" || p.USBPID != ""
	if err := validateProvision(p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !p.updatesWiFi() && !exists("/etc/wiibridge/wifi-provisioned") {
		http.Error(w, "Wi-Fi details are required until an initial profile is saved", http.StatusBadRequest)
		return
	}
	if p.AutoAttach && !p.Bridge && !exists("/etc/wiibridge/provisioned") {
		http.Error(w, "automatic attachment requires saved bridge settings", http.StatusBadRequest)
		return
	}

	c.provisionMu.Lock()
	defer c.provisionMu.Unlock()
	if err := stageProvision(p); err != nil {
		log.Printf("could not stage setup data: %v", err)
		http.Error(w, "could not stage setup data", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	output, err := c.runHelper(ctx, "provision")
	if err != nil {
		http.Error(w, "setup failed: "+strings.TrimSpace(string(output)), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>Setup saved</title>
<h1>Setup saved</h1><p>The credentials were validated and stored.</p>
<p>Existing Wi-Fi was retained if all three Wi-Fi fields were left blank.
Reboot to apply network or automatic-attachment changes. If Wi-Fi fails,
the setup access point will return automatically.</p>
<p><a href="/">Return to status</a></p>`)
}

func (p provisionRequest) updatesWiFi() bool {
	return p.WiFiCountry != "" || p.WiFiSSID != "" || p.WiFiPSK != ""
}

func validateProvision(p provisionRequest) error {
	if p.updatesWiFi() {
		if !countryPattern.MatchString(p.WiFiCountry) {
			return errors.New("Wi-Fi country must be a two-letter ISO country code")
		}
		if !ssidPattern.MatchString(p.WiFiSSID) {
			return errors.New("Wi-Fi SSID must contain 1-32 letters, numbers, spaces, dots, underscores, or hyphens")
		}
		if len(p.WiFiPSK) < 8 || len(p.WiFiPSK) > 63 || hasControl(p.WiFiPSK) {
			return errors.New("Wi-Fi password must contain 8-63 printable characters")
		}
	}
	if !p.Bridge {
		return nil
	}
	if !hostPattern.MatchString(p.NBDHost) {
		return errors.New("invalid NBD host")
	}
	port, err := strconv.Atoi(p.NBDPort)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("NBD port must be between 1 and 65535")
	}
	if !exportPattern.MatchString(p.NBDExport) {
		return errors.New("invalid NBD export name")
	}
	if (p.USBVID == "") != (p.USBPID == "") {
		return errors.New("USB VID and PID must be supplied together")
	}
	if p.USBVID != "" && (!usbIDPattern.MatchString(p.USBVID) || !usbIDPattern.MatchString(p.USBPID)) {
		return errors.New("USB VID and PID must use the form 0x1234")
	}
	if p.AutoAttach && p.USBVID == "" {
		return errors.New("automatic USB attachment requires an authorized USB VID and PID")
	}
	if p.TLSCA == "" || p.TLSCert == "" || p.TLSKey == "" {
		return errors.New("CA certificate, client certificate, and client key are required for bridge setup")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(p.TLSCA)) {
		return errors.New("invalid CA certificate")
	}
	if _, err := tls.X509KeyPair([]byte(p.TLSCert), []byte(p.TLSKey)); err != nil {
		return errors.New("client certificate and private key do not form a valid pair")
	}
	return nil
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func stageProvision(p provisionRequest) error {
	if err := os.MkdirAll(provisionRoot, 0o750); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(provisionRoot, ".provision-")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(tmp)
		}
	}()
	files := provisionFiles(p)
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(value), 0o600); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(provisionPath); err != nil {
		return err
	}
	if err := os.Rename(tmp, provisionPath); err != nil {
		return err
	}
	keep = true
	return nil
}

func provisionFiles(p provisionRequest) map[string]string {
	files := map[string]string{
		"wifi-update": "0",
		"bridge":      "0",
		"auto-attach": "0",
	}
	if p.updatesWiFi() {
		files["wifi-update"] = "1"
		files["wifi-country"] = p.WiFiCountry
		files["wifi-ssid"] = p.WiFiSSID
		files["wifi-password"] = p.WiFiPSK
	}
	if p.AutoAttach {
		files["auto-attach"] = "1"
	}
	if p.Bridge {
		files["bridge"] = "1"
		files["nbd-host"] = p.NBDHost
		files["nbd-port"] = p.NBDPort
		files["nbd-export"] = p.NBDExport
		files["tls-ca"] = p.TLSCA + "\n"
		files["tls-cert"] = p.TLSCert + "\n"
		files["tls-key"] = p.TLSKey + "\n"
		files["usb-vid"] = p.USBVID
		files["usb-pid"] = p.USBPID
	}
	return files
}

func csrfToken(token string) string {
	sum := sha256.Sum256([]byte("wiibridge-setup-csrf\x00" + token))
	return hex.EncodeToString(sum[:])
}

func validCSRF(r *http.Request, expected string) bool {
	return csrfMatches(r.Header.Get("X-WiiBridge-CSRF"), expected)
}

func csrfMatches(got, expected string) bool {
	return len(got) == len(expected) &&
		subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func (c *controller) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !c.limiter.allowed(host) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "too many authentication failures", http.StatusTooManyRequests)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if user, password, ok := r.BasicAuth(); ok && user == "admin" {
			got = password
		}
		if len(got) != len(c.token) ||
			subtle.ConstantTimeCompare([]byte(got), []byte(c.token)) != 1 {
			c.limiter.failed(host)
			w.Header().Set("WWW-Authenticate", `Basic realm="WiiBridge Bridge", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		c.limiter.succeeded(host)
		next(w, r)
	}
}

type authAttempt struct {
	failures int
	start    time.Time
}

type authLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hosts  map[string]authAttempt
}

func newAuthLimiter(limit int, window time.Duration) *authLimiter {
	return &authLimiter{limit: limit, window: window, hosts: make(map[string]authAttempt)}
}

func (l *authLimiter) allowed(host string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, ok := l.hosts[host]
	if !ok || time.Since(attempt.start) >= l.window {
		delete(l.hosts, host)
		return true
	}
	return attempt.failures < l.limit
}

func (l *authLimiter) failed(host string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt, ok := l.hosts[host]
	if !ok || time.Since(attempt.start) >= l.window {
		attempt = authAttempt{start: time.Now()}
	}
	attempt.failures++
	l.hosts[host] = attempt
}

func (l *authLimiter) succeeded(host string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hosts, host)
}

func collect(target string) status {
	boardBytes, _ := os.ReadFile("/proc/device-tree/model")
	board := strings.TrimRight(string(boardBytes), "\x00\n")
	ok := boardCompatible(target, board)
	state := "ready"
	if !ok {
		state = "wrong-board-recovery"
	} else if !exists("/etc/wiibridge/wifi-provisioned") {
		state = "setup"
	} else if !exists("/etc/wiibridge/provisioned") {
		state = "network-ready"
	}
	usbController, usbState := usbControllerState()
	exportModeBytes, _ := os.ReadFile("/run/wiibridge/export-mode")
	return status{
		Target:        target,
		Board:         board,
		BoardOK:       ok,
		Provisioned:   exists("/etc/wiibridge/provisioned"),
		WiFiReady:     exists("/etc/wiibridge/wifi-provisioned"),
		AutoAttach:    exists("/etc/wiibridge/auto-attach"),
		NBDConnected:  exists("/run/wiibridge/nbd-connected"),
		USBAttached:   exists("/run/wiibridge/usb-attached"),
		ExportMode:    strings.TrimSpace(string(exportModeBytes)),
		USBController: usbController,
		USBState:      usbState,
		Addresses:     localAddresses(),
		State:         state,
		Compatibility: firmwareDescriptor(target, board),
	}
}

func firmwareDescriptor(target, board string) compat.Descriptor {
	deviceID := ""
	if data, err := os.ReadFile("/etc/machine-id"); err == nil {
		sum := sha256.Sum256([]byte(strings.TrimSpace(string(data))))
		deviceID = hex.EncodeToString(sum[:16])
	}
	descriptor := compat.NewDescriptor(
		"firmware", board, deviceID, productVersion, gitCommit, buildTime,
		compat.FirmwareCapabilities(),
	)
	descriptor.BuildDirty = buildDirty == "true"
	return descriptor
}

func (c *controller) recordAction(action string, actionErr error) {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()
	if actionErr != nil {
		c.actionErrors++
		return
	}
	switch action {
	case "attach":
		c.attachCount++
	case "connect", "connect-wii", "connect-gamecube-physical",
		"connect-gamecube-emulated":
		c.reconnects++
	}
	// Force the next authenticated metrics request to refresh system state.
	c.metricsAt = time.Time{}
}

func (c *controller) collectMetrics() perf.PiSnapshot {
	c.metricsMu.Lock()
	defer c.metricsMu.Unlock()
	if !c.metricsAt.IsZero() && time.Since(c.metricsAt) < 10*time.Second {
		return c.metrics
	}
	started := time.Now()
	status := collect(c.target)
	if observedUSBReset(c.lastUSBState, status.USBState, status.USBAttached) {
		c.usbResets++
	}
	c.lastUSBState = status.USBState
	result := perf.PiSnapshot{
		UpdatedAt: time.Now().UTC(), Available: true, BoardModel: status.Board,
		NBDState:          yesState(status.NBDConnected, "connected", "disconnected"),
		NBDReconnectCount: c.reconnects, USBState: status.USBState,
		USBAttachCount: c.attachCount, USBResetCount: c.usbResets,
		ActiveProfile: status.ExportMode,
		RecentErrors:  c.actionErrors,
	}
	if values := readNumbers("/proc/uptime", 1); len(values) == 1 {
		result.FirmwareUptimeSeconds = int64(values[0])
	}
	if values := readNumbers("/proc/loadavg", 3); len(values) == 3 {
		result.Load1, result.Load5, result.Load15 = values[0], values[1], values[2]
	}
	result.MemoryTotalBytes, result.MemoryUsedBytes = memoryUsage()
	if value, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp"); err == nil {
		if milli, parseErr := strconv.ParseFloat(strings.TrimSpace(string(value)), 64); parseErr == nil {
			result.TemperatureCelsius = milli / 1000
		}
	}
	total, idle := cpuTimes()
	if c.cpuTotal != 0 && total > c.cpuTotal {
		deltaTotal, deltaIdle := total-c.cpuTotal, idle-c.cpuIdle
		if deltaTotal != 0 && deltaIdle <= deltaTotal {
			result.CPUUtilizationPercent = float64(deltaTotal-deltaIdle) * 100 / float64(deltaTotal)
		}
	}
	c.cpuTotal, c.cpuIdle = total, idle
	result.NetworkInterface, result.LinkState = primaryInterface()
	result.WiFiSignalDBM = wifiSignal(result.NetworkInterface)
	if info, err := os.Stat("/dev/nbd0"); err == nil && info.Mode()&os.ModeDevice != 0 {
		if sectors, readErr := os.ReadFile("/sys/class/block/nbd0/size"); readErr == nil {
			if value, parseErr := strconv.ParseInt(strings.TrimSpace(string(sectors)), 10, 64); parseErr == nil {
				result.CurrentBlockDeviceSize = value * 512
			}
		}
		if stat, readErr := os.ReadFile("/sys/class/block/nbd0/stat"); readErr == nil {
			fields := strings.Fields(string(stat))
			if len(fields) >= 3 {
				reads, readsErr := strconv.ParseUint(fields[0], 10, 64)
				sectors, sectorsErr := strconv.ParseUint(fields[2], 10, 64)
				if readsErr == nil {
					result.NBDRequestsCompleted = reads
				}
				if sectorsErr == nil && !c.nbdSample.IsZero() &&
					sectors >= c.nbdSectors {
					seconds := time.Since(c.nbdSample).Seconds()
					if seconds > 0 {
						result.NBDReadBytesPerSecond =
							float64((sectors-c.nbdSectors)*512) / seconds
					}
				}
				if sectorsErr == nil {
					c.nbdSectors, c.nbdSample = sectors, time.Now()
				}
			}
		}
	}
	result.CollectionDurationUS = time.Since(started).Microseconds()
	c.metricsAt, c.metrics = time.Now(), result
	return result
}

func observedUSBReset(previous, current string, attached bool) bool {
	return previous == "configured" && attached && current != "configured"
}

func yesState(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}

func readNumbers(path string, maximum int) []float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(data))
	if len(fields) > maximum {
		fields = fields[:maximum]
	}
	result := make([]float64, 0, len(fields))
	for _, field := range fields {
		value, parseErr := strconv.ParseFloat(field, 64)
		if parseErr != nil {
			break
		}
		result = append(result, value)
	}
	return result
}

func memoryUsage() (int64, int64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer file.Close()
	var total, available int64
	scanner := bufio.NewScanner(io.LimitReader(file, 64<<10))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, _ := strconv.ParseInt(fields[1], 10, 64)
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			total = value << 10
		case "MemAvailable":
			available = value << 10
		}
	}
	return total, max(total-available, 0)
}

func cpuTimes() (uint64, uint64) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0
	}
	line, _, _ := strings.Cut(string(data), "\n")
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0
	}
	var total uint64
	var values []uint64
	for _, field := range fields[1:] {
		value, parseErr := strconv.ParseUint(field, 10, 64)
		if parseErr != nil {
			return 0, 0
		}
		total += value
		values = append(values, value)
	}
	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return total, idle
}

func primaryInterface() (string, string) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", "unavailable"
	}
	for _, candidate := range interfaces {
		if candidate.Flags&net.FlagLoopback != 0 || candidate.Flags&net.FlagUp == 0 {
			continue
		}
		state := "up"
		if value, readErr := os.ReadFile(
			filepath.Join("/sys/class/net", candidate.Name, "operstate")); readErr == nil {
			state = strings.TrimSpace(string(value))
		}
		return candidate.Name, state
	}
	return "", "down"
}

func wifiSignal(name string) int {
	if name == "" {
		return 0
	}
	data, err := os.ReadFile("/proc/net/wireless")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || strings.TrimSuffix(fields[0], ":") != name {
			continue
		}
		value, _ := strconv.ParseFloat(strings.TrimSuffix(fields[3], "."), 64)
		return int(value)
	}
	return 0
}

func usbControllerState() (string, string) {
	controllers, err := os.ReadDir("/sys/class/udc")
	if err != nil || len(controllers) == 0 {
		return "none", "unavailable"
	}
	name := controllers[0].Name()
	value, err := os.ReadFile(filepath.Join("/sys/class/udc", name, "state"))
	if err != nil {
		return name, "unknown"
	}
	return name, strings.TrimSpace(string(value))
}

func localAddresses() []string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		value := address.String()
		if strings.HasPrefix(value, "127.") || value == "::1/128" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func boardCompatible(target, board string) bool {
	switch target {
	case "zero-w-armhf":
		return strings.Contains(board, "Zero W") && !strings.Contains(board, "Zero 2")
	case "pi4-arm64":
		return strings.Contains(board, "Raspberry Pi 4 Model B")
	case "pi5-arm64":
		return strings.Contains(board, "Raspberry Pi 5")
	default:
		return false
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>WiiBridge Bridge</title>
<style>body{font:16px system-ui;max-width:48rem;margin:2rem auto;padding:0 1rem}dt{font-weight:700}dd{margin-bottom:.6rem}form{display:inline}button,a.button{display:inline-block;margin:.25rem;padding:.6rem .8rem}</style>
<h1>WiiBridge Bridge</h1>
<dl>
<dt>Target</dt><dd>{{.Status.Target}}</dd>
<dt>Detected board</dt><dd>{{.Status.Board}}</dd>
<dt>State</dt><dd>{{.Status.State}}</dd>
<dt>Addresses</dt><dd>{{range .Status.Addresses}}{{.}}<br>{{else}}none{{end}}</dd>
<dt>Wi-Fi configured</dt><dd>{{.Status.WiFiReady}}</dd>
<dt>Bridge configured</dt><dd>{{.Status.Provisioned}}</dd>
<dt>Automatic attachment</dt><dd>{{.Status.AutoAttach}}</dd>
<dt>NBD connected</dt><dd>{{.Status.NBDConnected}}</dd>
<dt>USB attached</dt><dd>{{.Status.USBAttached}}</dd>
<dt>Export mode</dt><dd>{{if .Status.ExportMode}}{{.Status.ExportMode}}{{else}}disconnected{{end}}</dd>
<dt>USB controller</dt><dd>{{.Status.USBController}}</dd>
<dt>USB link state</dt><dd>{{.Status.USBState}}</dd>
</dl>
<p><a class="button" href="/setup">Network and bridge setup</a></p>
<h2>Bridge actions</h2>
{{range .Actions}}<form method="post" action="/action/{{.Name}}">
<input type="hidden" name="csrf" value="{{$.CSRF}}">
<button type="submit">{{.Label}}</button></form>{{end}}
<p>API actions require the <code>X-WiiBridge-CSRF: {{.CSRF}}</code> header.</p>
</html>`))

var setupTemplate = template.Must(template.New("setup").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>WiiBridge setup</title>
<style>body{font:16px system-ui;max-width:48rem;margin:2rem auto;padding:0 1rem}label{display:block;font-weight:700;margin-top:1rem}input,textarea{box-sizing:border-box;width:100%;padding:.55rem}input.checkbox{width:auto}textarea{min-height:8rem}button{margin-top:1.5rem;padding:.7rem 1rem}.note{background:#eef;padding:1rem}</style>
<h1>WiiBridge setup</h1>
<p class="note">Wi-Fi is {{if .WiFiReady}}configured{{else}}not configured{{end}}.
The NBD bridge is {{if .Provisioned}}configured{{else}}not configured{{end}}.
{{if .WiFiReady}}Leave all three Wi-Fi fields blank to keep the saved network.
Enter all three only to change it.{{else}}All three Wi-Fi fields are required for
initial setup.{{end}} Leaving every bridge field blank preserves existing bridge
settings. Reboot after saving network or automatic-attachment changes.</p>
<form method="post" action="/setup">
<input type="hidden" name="csrf" value="{{.CSRF}}">
<h2>2.4 GHz Wi-Fi</h2>
<label>Two-letter country code <input name="wifi_country" value="{{if not .WiFiReady}}US{{end}}" maxlength="2" {{if not .WiFiReady}}required{{end}}></label>
<label>SSID <input name="wifi_ssid" maxlength="32" {{if not .WiFiReady}}required{{end}}></label>
<label>Password <input name="wifi_password" type="password" minlength="8" maxlength="63" {{if not .WiFiReady}}required{{end}}></label>
<h2>Authenticated NBD/TLS bridge (optional as a complete group)</h2>
<label>Server host <input name="nbd_host"></label>
<label>Server port <input name="nbd_port" inputmode="numeric" value="10809"></label>
<label>Export name <input name="nbd_export"></label>
<label>CA certificate (PEM) <textarea name="tls_ca" spellcheck="false"></textarea></label>
<label>Client certificate (PEM) <textarea name="tls_cert" spellcheck="false"></textarea></label>
<label>Unencrypted client private key (PEM) <textarea name="tls_key" spellcheck="false"></textarea></label>
<label>Authorized USB VID (not auto-detected), in the form 0x1234 <input name="usb_vid"></label>
<label>Authorized USB PID (not auto-detected), in the form 0xabcd <input name="usb_pid"></label>
<label><input class="checkbox" name="auto_attach" type="checkbox" value="1" {{if .AutoAttach}}checked{{end}}>
Automatically validate, connect, and attach USB after boot (requires authorized VID/PID)</label>
<button type="submit">Validate and save</button>
</form>
<p><a href="/">Return to status</a></p>
</html>`))
