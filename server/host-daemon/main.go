package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"wiibridge/server/host-daemon/bridgecontrol"
	"wiibridge/server/host-daemon/exportprofile"
	"wiibridge/server/host-daemon/gamecube"
	"wiibridge/server/host-daemon/scanner"
	"wiibridge/server/host-daemon/store"
	"wiibridge/server/host-daemon/vdisk"
	"wiibridge/server/nbd-plugin"
	"wiibridge/shared/model"
)

const version = "0.1.0-rc.1"

type app struct {
	mu       sync.RWMutex
	switchMu sync.Mutex
	root     string
	dataDir  string
	disk     *vdisk.Disk
	scan     scanner.Result
	tokenSum [32]byte
	started  time.Time
	store    *store.Store
	gcScan   gamecube.Result
	exports  *exportprofile.Manager
	wii      *wiiExportProfile
	csrf     string
	imports  map[string]importJob
	activeGC *gamecube.VolumeManifest
	authMu   sync.Mutex
	failures map[string]authFailure
	pi       piController
}

type piController interface {
	Action(context.Context, string) error
	Status(context.Context) (bridgecontrol.Status, error)
	Address() string
	SetAddress(context.Context, string) error
}

type authFailure struct {
	count int
	reset time.Time
}

type importJob struct {
	Status   string                   `json:"status"`
	Error    string                   `json:"error,omitempty"`
	Manifest *gamecube.VolumeManifest `json:"manifest,omitempty"`
}

type wiiExportProfile struct{ app *app }

func (p *wiiExportProfile) Platform() string { return "wii" }
func (p *wiiExportProfile) Backend() nbd.Backend {
	p.app.mu.RLock()
	defer p.app.mu.RUnlock()
	return p.app.disk
}
func (p *wiiExportProfile) ReadOnly() bool { return true }
func (p *wiiExportProfile) Validate() error {
	if p.Backend() == nil {
		return errors.New("Wii export is unavailable")
	}
	return nil
}
func (p *wiiExportProfile) Close() error { return nil }

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "scan":
			scanCLI(os.Args[2:])
			return
		case "healthcheck":
			healthCLI(os.Args[2:])
			return
		case "version":
			fmt.Println(version)
			return
		}
	}
	if err := serve(); err != nil {
		slog.Error("host stopped", "error", err)
		os.Exit(1)
	}
}

func scanCLI(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	root := fs.String("library", "/library", "read-only WBFS library")
	_ = fs.Parse(args)
	result, err := scanner.Scan(*root)
	if err != nil {
		slog.Error("scan failed", "error", err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}

func healthCLI(args []string) {
	fs := flag.NewFlagSet("healthcheck", flag.ExitOnError)
	url := fs.String("url", "https://127.0.0.1:8445/healthz", "health URL")
	caPath := fs.String("ca", "/certs/ca.crt", "CA certificate")
	_ = fs.Parse(args)
	pem, err := os.ReadFile(*caPath)
	if err != nil {
		os.Exit(1)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		os.Exit(1)
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13},
	}}
	resp, err := client.Get(*url)
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	resp.Body.Close()
}

func serve() error {
	root := env("WIIBRIDGE_LIBRARY", "/library")
	dataDir := env("WIIBRIDGE_DATA", "/data")
	token := os.Getenv("WIIBRIDGE_ADMIN_TOKEN")
	if len(token) < 20 {
		return errors.New("WIIBRIDGE_ADMIN_TOKEN must contain at least 20 characters")
	}
	if err := assertReadOnly(root); err != nil {
		return fmt.Errorf("library safety check: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "snapshots"), 0o700); err != nil {
		return err
	}
	result, err := scanner.Scan(root)
	if err != nil {
		return err
	}
	disk, err := vdisk.Build("all", result.Games, version)
	if err != nil {
		return err
	}
	gcResult, err := gamecube.Scan(root)
	if err != nil {
		return err
	}
	database, err := store.Open(filepath.Join(dataDir, "wiibridge.sqlite3"))
	if err != nil {
		return err
	}
	defer database.Close()
	csrfSum := sha256.Sum256([]byte("wiibridge-host-csrf\x00" + token))
	a := &app{root: root, dataDir: dataDir, disk: disk, scan: result, gcScan: gcResult,
		tokenSum: sha256.Sum256([]byte(token)), started: time.Now(), store: database,
		failures: make(map[string]authFailure), csrf: hex.EncodeToString(csrfSum[:]),
		imports: make(map[string]importJob)}
	piManager, err := bridgecontrol.NewManager(
		os.Getenv("WIIBRIDGE_PI_URL"),
		os.Getenv("WIIBRIDGE_PI_ADMIN_TOKEN"),
		os.Getenv("WIIBRIDGE_PI_CERT"),
		filepath.Join(dataDir, "pi-address"),
	)
	if err != nil {
		return fmt.Errorf("automatic Pi switching configuration: %w", err)
	}
	a.pi = configuredPiController(piManager)
	a.wii = &wiiExportProfile{app: a}
	a.exports, err = exportprofile.New(a.wii)
	if err != nil {
		return err
	}
	if err := a.persistSnapshot(); err != nil {
		return err
	}
	tlsConfig, err := mutualTLSConfig(
		env("WIIBRIDGE_TLS_CERT", "/certs/server.crt"),
		env("WIIBRIDGE_TLS_KEY", "/certs/server.key"),
		env("WIIBRIDGE_TLS_CLIENT_CA", "/certs/clients-ca.crt"),
	)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if piManager != nil {
		go piManager.Run(ctx)
	}
	nbdListener, err := net.Listen("tcp", env("WIIBRIDGE_NBD_LISTEN", ":10809"))
	if err != nil {
		return err
	}
	nbdServer := &nbd.Server{
		BackendAcquirer: a.exports.BeginSession,
		TLS:             tlsConfig, ExportName: env("WIIBRIDGE_EXPORT", "all"),
		Deadline: 30 * time.Second, MaxRequest: 1 << 20,
	}
	errs := make(chan error, 2)
	go func() { errs <- nbdServer.Serve(ctx, nbdListener) }()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/status", a.auth(a.status))
	mux.HandleFunc("GET /api/v1/pi/status", a.auth(a.piStatus))
	mux.HandleFunc("POST /api/v1/pi/address", a.auth(a.setPiAddress))
	mux.HandleFunc("GET /assets/pi-status.js", a.auth(piStatusScript))
	mux.HandleFunc("GET /api/v1/scan", a.auth(a.scanResult))
	mux.HandleFunc("POST /api/v1/scan", a.auth(a.rescan))
	mux.HandleFunc("GET /api/v1/gamecube", a.auth(a.gamecubeResult))
	mux.HandleFunc("GET /api/v1/gamecube/imports", a.auth(a.gamecubeImports))
	mux.HandleFunc("POST /api/v1/gamecube/import", a.auth(a.importGameCube))
	mux.HandleFunc("POST /api/v1/gamecube/settings", a.auth(a.saveGameCubeSettings))
	mux.HandleFunc("POST /api/v1/gamecube/saves/restore", a.auth(a.restoreGameCubeSave))
	mux.HandleFunc("POST /api/v1/export/{platform}", a.auth(a.selectExport))
	mux.HandleFunc("POST /api/v1/pi/{action}", a.auth(a.piPowerAction))
	mux.HandleFunc("GET /metrics", a.auth(a.metrics))
	mux.HandleFunc("GET /", a.auth(a.dashboard))
	web := &http.Server{
		Addr: env("WIIBRIDGE_HTTPS_LISTEN", ":8445"), Handler: securityHeaders(mux),
		TLSConfig: tlsConfig.Clone(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10,
	}
	web.TLSConfig.ClientAuth = tls.NoClientCert
	web.TLSConfig.MinVersion = tls.VersionTLS13
	web.TLSConfig.MaxVersion = 0
	go func() {
		errs <- web.ListenAndServeTLS(
			env("WIIBRIDGE_TLS_CERT", "/certs/server.crt"),
			env("WIIBRIDGE_TLS_KEY", "/certs/server.key"),
		)
	}()
	select {
	case <-ctx.Done():
	case err = <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			cancel()
		}
	}
	shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	_ = web.Shutdown(shutdown)
	_ = nbdListener.Close()
	return err
}

// configuredPiController prevents a nil *Manager from becoming a non-nil
// interface value. That distinction matters because dashboard feature checks
// use the interface to determine whether Pi coordination is enabled.
func configuredPiController(manager *bridgecontrol.Manager) piController {
	if manager == nil {
		return nil
	}
	return manager
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func assertReadOnly(path string) error {
	probe := filepath.Join(path, ".wiibridge-write-probe-"+strconv.Itoa(os.Getpid()))
	f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		f.Close()
		_ = os.Remove(probe)
		return errors.New("library mount is writable; refusing startup")
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EROFS) {
		return nil
	}
	return fmt.Errorf("cannot prove read-only mount: %w", err)
}

func mutualTLSConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("client CA contains no certificates")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert}, ClientCAs: pool,
		// libnbd 1.22/GnuTLS 3.8.9 does not select an X.509 client
		// certificate when Go requests it during a TLS 1.3 handshake.  TLS
		// 1.2 retains mutual authentication and is the interop-tested profile.
		ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12,
	}, nil
}

func (a *app) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host == "" {
			host = r.RemoteAddr
		}
		if a.authLimited(host, false) {
			http.Error(w, "authentication rate limit", http.StatusTooManyRequests)
			return
		}
		value := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if user, password, ok := r.BasicAuth(); ok && user == "admin" {
			value = password
		}
		sum := sha256.Sum256([]byte(value))
		if len(value) == 0 || subtle.ConstantTimeCompare(sum[:], a.tokenSum[:]) != 1 {
			a.authLimited(host, true)
			w.Header().Set("WWW-Authenticate", `Basic realm="WiiBridge", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		confirmed := r.Header.Get("X-WiiBridge-CSRF") == "1"
		if !confirmed && r.Method != http.MethodGet && r.Method != http.MethodHead {
			_ = r.ParseForm()
			confirmed = subtle.ConstantTimeCompare([]byte(r.Form.Get("csrf")), []byte(a.csrf)) == 1
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !confirmed {
			http.Error(w, "missing CSRF confirmation", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (a *app) authLimited(host string, failed bool) bool {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	now := time.Now()
	entry := a.failures[host]
	if now.After(entry.reset) {
		entry = authFailure{reset: now.Add(time.Minute)}
	}
	if failed {
		entry.count++
		a.failures[host] = entry
		if len(a.failures) > 4096 {
			clear(a.failures)
		}
	}
	return entry.count >= 10
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.disk == nil {
		http.Error(w, "unhealthy", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"status": "healthy", "version": version})
}

func (a *app) status(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	snapshot := a.disk.Snapshot()
	wiiGames := len(a.scan.Games)
	wiiRejected := len(a.scan.Rejected)
	gameCubeGames := len(a.gcScan.Games)
	gameCubeRejected := len(a.gcScan.Rejected)
	started := a.started
	a.mu.RUnlock()
	writeJSON(w, map[string]any{
		"version": version, "snapshot": snapshot, "games": wiiGames,
		"rejected": wiiRejected, "gamecube_games": gameCubeGames,
		"gamecube_rejected": gameCubeRejected, "platform": a.exports.Platform(),
		"export_state": a.exports.State(), "automatic_switching": a.pi != nil,
		"uptime_seconds": int(time.Since(started).Seconds()),
	})
}

func (a *app) piStatus(w http.ResponseWriter, r *http.Request) {
	if a.pi == nil {
		http.Error(w, "Pi connection is not configured", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	status, err := a.pi.Status(ctx)
	if err != nil {
		writeJSONStatus(w, http.StatusBadGateway, map[string]any{
			"connected": false, "error": "Pi status unavailable",
		})
		return
	}
	writeJSON(w, map[string]any{"connected": true, "pi": status})
}

func (a *app) setPiAddress(w http.ResponseWriter, r *http.Request) {
	if a.pi == nil {
		http.Error(w, "Pi connection is not configured", http.StatusServiceUnavailable)
		return
	}
	address := strings.TrimSpace(r.FormValue("address"))
	if net.ParseIP(address) == nil {
		http.Error(w, "Pi address must be a literal IPv4 or IPv6 address", http.StatusBadRequest)
		return
	}
	if err := a.pi.SetAddress(r.Context(), address); err != nil {
		slog.Warn("cannot update Pi address", "error", err)
		http.Error(w, "Pi address update failed", http.StatusInternalServerError)
		return
	}
	respondAction(w, r, http.StatusOK,
		map[string]string{"status": "updated", "address": a.pi.Address()},
		"Raspberry Pi address updated; live status is reconnecting.", "all")
}

func piStatusScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write([]byte(`(() => {
  const panel = document.getElementById("pi-live");
  if (!panel) return;
  const text = (id, value) => {
    const element = document.getElementById(id);
    if (element) element.textContent = value;
  };
  const yes = (value, enabled, disabled) => value ? enabled : disabled;
  async function refresh() {
    const dot = document.getElementById("pi-dot");
    try {
      const response = await fetch(panel.dataset.statusUrl, {
        headers: {"Accept": "application/json"},
        cache: "no-store"
      });
      if (!response.ok) throw new Error("status unavailable");
      const value = await response.json();
      const pi = value.pi;
      dot.classList.remove("offline");
      text("pi-connection", "Connected");
      text("pi-state", pi.state || "unknown");
      text("pi-export", pi.export_mode || "not selected");
      text("pi-storage", yes(pi.nbd_connected, "NBD connected", "NBD disconnected") +
        " · " + yes(pi.usb_attached, "USB attached", "USB detached"));
      text("pi-usb", (pi.usb_controller || "none") + " · " + (pi.usb_state || "unknown"));
      text("pi-board", pi.detected_board || "unknown");
      text("pi-addresses", (pi.addresses || []).join(", ") || "none");
      text("pi-provision", yes(pi.provisioned, "Ready", "Incomplete"));
      text("pi-attach", yes(pi.auto_attach, "Enabled", "Disabled"));
      text("pi-updated", "Live · " + new Date().toLocaleTimeString());
    } catch (_) {
      dot.classList.add("offline");
      text("pi-connection", "Unavailable");
      text("pi-updated", "Retrying");
    }
  }
  refresh();
  window.setInterval(refresh, 10000);
})();`))
}

func (a *app) scanResult(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	writeJSON(w, a.scan)
}

func (a *app) gamecubeResult(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	writeJSON(w, a.gcScan)
}

func (a *app) gamecubeImports(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	writeJSON(w, a.imports)
}

func (a *app) rescan(w http.ResponseWriter, r *http.Request) {
	result, err := scanner.Scan(a.root)
	if err != nil {
		http.Error(w, "scan failed", http.StatusInternalServerError)
		return
	}
	disk, err := vdisk.Build("all", result.Games, version)
	if err != nil {
		http.Error(w, "snapshot build failed", http.StatusInternalServerError)
		return
	}
	gcResult, err := gamecube.Scan(a.root)
	if err != nil {
		http.Error(w, "GameCube scan failed", http.StatusInternalServerError)
		return
	}
	a.mu.Lock()
	a.scan, a.disk, a.gcScan = result, disk, gcResult
	a.mu.Unlock()
	if err := a.persistSnapshot(); err != nil {
		http.Error(w, "snapshot persistence failed", http.StatusInternalServerError)
		return
	}
	respondAction(w, r, http.StatusOK, disk.Snapshot(),
		"Library rescan completed.", "all")
}

func (a *app) findGameCube(id string, revision byte) (gamecube.Game, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, game := range a.gcScan.Games {
		if game.ID == id && game.Revision == revision {
			return game, true
		}
	}
	return gamecube.Game{}, false
}

func (a *app) importGameCube(w http.ResponseWriter, r *http.Request) {
	id := strings.ToUpper(strings.TrimSpace(r.FormValue("id")))
	revision64, err := strconv.ParseUint(r.FormValue("revision"), 10, 8)
	if err != nil {
		http.Error(w, "invalid revision", http.StatusBadRequest)
		return
	}
	game, ok := a.findGameCube(id, byte(revision64))
	if !ok {
		http.Error(w, "validated GameCube game not found", http.StatusNotFound)
		return
	}
	settings, err := gamecube.LoadSettings(filepath.Join(a.dataDir, "gamecube"), id, byte(revision64))
	if err != nil {
		http.Error(w, "settings invalid", http.StatusConflict)
		return
	}
	key := gamecube.CacheKey(game, settings.MemoryCard)
	a.mu.Lock()
	if job, exists := a.imports[key]; exists && (job.Status == "queued" || job.Status == "building") {
		a.mu.Unlock()
		http.Error(w, "import already active", http.StatusConflict)
		return
	}
	a.imports[key] = importJob{Status: "queued"}
	a.mu.Unlock()
	go func() {
		a.mu.Lock()
		a.imports[key] = importJob{Status: "building"}
		a.mu.Unlock()
		manifest, buildErr := gamecube.BuildVolume(context.Background(),
			filepath.Join(a.dataDir, "gamecube", "cache"), game, settings.MemoryCard)
		a.mu.Lock()
		defer a.mu.Unlock()
		if buildErr != nil {
			a.imports[key] = importJob{Status: "failed", Error: buildErr.Error()}
			return
		}
		a.imports[key] = importJob{Status: "ready", Manifest: &manifest}
	}()
	respondAction(w, r, http.StatusAccepted,
		map[string]string{"cache_key": key, "status": "queued"},
		"GameCube export preparation was queued.", "gamecube")
}

func (a *app) saveGameCubeSettings(w http.ResponseWriter, r *http.Request) {
	id := strings.ToUpper(strings.TrimSpace(r.FormValue("id")))
	revision64, err := strconv.ParseUint(r.FormValue("revision"), 10, 8)
	if err != nil {
		http.Error(w, "invalid revision", http.StatusBadRequest)
		return
	}
	if _, ok := a.findGameCube(id, byte(revision64)); !ok {
		http.Error(w, "validated GameCube game not found", http.StatusNotFound)
		return
	}
	settings := gamecube.DefaultSettings(id, byte(revision64))
	settings.MemoryCard = gamecube.MemoryCardMode(r.FormValue("memory_card"))
	size, err := strconv.ParseUint(r.FormValue("memory_card_size"), 10, 8)
	if err != nil {
		http.Error(w, "invalid memory-card size", http.StatusBadRequest)
		return
	}
	settings.MemoryCardSize = byte(size)
	settings.MultiCard = r.FormValue("multi_card") == "1"
	settings.NativeControl = r.FormValue("native_control") == "1"
	settings.VideoMode = r.FormValue("video_mode")
	settings.Progressive = r.FormValue("progressive") == "1"
	settings.PAL50Patch = r.FormValue("pal50_patch") == "1"
	settings.Widescreen = r.FormValue("widescreen") == "1"
	if value := r.FormValue("video_width"); value != "" {
		settings.VideoWidth, err = strconv.Atoi(value)
		if err != nil {
			http.Error(w, "invalid video width", http.StatusBadRequest)
			return
		}
	}
	if value := r.FormValue("video_offset"); value != "" {
		settings.VideoOffset, err = strconv.Atoi(value)
		if err != nil {
			http.Error(w, "invalid video offset", http.StatusBadRequest)
			return
		}
	}
	settings.Cheats = r.FormValue("cheats") == "1"
	settings.UseIPL = r.FormValue("use_ipl") == "1"
	settings.ControllerMode = r.FormValue("controller_mode")
	settings.DiscSpeed = r.FormValue("disc_speed")
	settings.ReturnToLoader = r.FormValue("return_to_loader") == "1"
	if err = gamecube.SaveSettings(filepath.Join(a.dataDir, "gamecube"), settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	respondAction(w, r, http.StatusOK, settings,
		"Compatibility settings saved.", "gamecube")
}

func (a *app) selectExport(w http.ResponseWriter, r *http.Request) {
	a.switchMu.Lock()
	defer a.switchMu.Unlock()
	platform := r.PathValue("platform")
	if platform != "wii" && platform != "gamecube" {
		http.Error(w, "unsupported platform", http.StatusBadRequest)
		return
	}
	var next exportprofile.Profile = a.wii
	var selectedManifest *gamecube.VolumeManifest
	connectAction := "connect-wii"
	if platform == "gamecube" {
		id := strings.ToUpper(strings.TrimSpace(r.FormValue("id")))
		revision64, err := strconv.ParseUint(r.FormValue("revision"), 10, 8)
		if err != nil {
			http.Error(w, "invalid revision", http.StatusBadRequest)
			return
		}
		game, ok := a.findGameCube(id, byte(revision64))
		if !ok {
			http.Error(w, "validated GameCube game not found", http.StatusNotFound)
			return
		}
		settings, err := gamecube.LoadSettings(filepath.Join(a.dataDir, "gamecube"), id, byte(revision64))
		if err != nil {
			http.Error(w, "settings invalid", http.StatusConflict)
			return
		}
		manifest, manifestPath, err := gamecube.FindReadyVolume(
			filepath.Join(a.dataDir, "gamecube", "cache"), game, settings.MemoryCard)
		if err != nil {
			http.Error(w, "GameCube cache is not ready; use Prepare export first",
				http.StatusConflict)
			return
		}
		backend, err := gamecube.OpenFileBackend(manifest.ImagePath,
			settings.MemoryCard == gamecube.MemoryCardEmulated)
		if err != nil {
			http.Error(w, "GameCube backend open failed", http.StatusInternalServerError)
			return
		}
		next = &exportprofile.BasicProfile{
			Name: "gamecube", BlockBackend: backend,
			Immutable: settings.MemoryCard == gamecube.MemoryCardPhysical,
			ValidateProfile: func() error {
				_, validateErr := gamecube.LoadAndValidateVolume(manifestPath)
				return validateErr
			},
			CloseProfile: backend.Close,
		}
		selectedManifest = &manifest
		if settings.MemoryCard == gamecube.MemoryCardEmulated {
			connectAction = "connect-gamecube-emulated"
		} else {
			connectAction = "connect-gamecube-physical"
		}
	}
	a.mu.RLock()
	previousManifest := a.activeGC
	a.mu.RUnlock()
	automatic := a.pi != nil
	if automatic {
		if err := a.pi.Action(r.Context(), "detach"); err != nil {
			_ = next.Close()
			http.Error(w, "automatic switch stopped before changing the host export: "+err.Error(),
				http.StatusBadGateway)
			return
		}
		if err := a.pi.Action(r.Context(), "disconnect"); err != nil {
			_ = next.Close()
			http.Error(w, "automatic switch left USB detached: "+err.Error(),
				http.StatusBadGateway)
			return
		}
	}
	if err := a.waitForExportDisconnect(r.Context()); err != nil {
		if next != a.wii {
			_ = next.Close()
		}
		if automatic {
			http.Error(w, "Pi disconnected but the host still has active I/O; USB remains detached: "+
				err.Error(), http.StatusConflict)
		} else {
			http.Error(w, "disconnect Pi NBD session before switching: "+err.Error(),
				http.StatusConflict)
		}
		return
	}
	if err := a.exports.Select(next); err != nil {
		_ = next.Close()
		http.Error(w, "export selection failed: "+err.Error(), http.StatusConflict)
		return
	}
	a.mu.Lock()
	a.activeGC = selectedManifest
	a.mu.Unlock()
	if platform == "wii" && previousManifest != nil {
		if _, err := gamecube.BackupMemoryCards(*previousManifest,
			filepath.Join(a.dataDir, "gamecube", "save-backups"),
			gamecube.DefaultSaveBackupRetention); err != nil {
			http.Error(w, "Wii mode selected, but GameCube save backup failed: "+err.Error(),
				http.StatusInternalServerError)
			return
		}
	}
	if automatic {
		if err := a.pi.Action(r.Context(), connectAction); err != nil {
			a.safePiDisconnect()
			http.Error(w, "host export selected, but Pi reconnect failed; USB remains detached: "+
				err.Error(), http.StatusBadGateway)
			return
		}
		if err := a.pi.Action(r.Context(), "attach"); err != nil {
			a.safePiDisconnect()
			http.Error(w, "Pi connected, but USB attach failed and was rolled back: "+
				err.Error(), http.StatusBadGateway)
			return
		}
	}
	notice := "Host export switched to " + platform + "."
	if automatic {
		notice = "Pi safely detached, switched to " + platform + ", reconnected, and reattached USB."
	}
	respondAction(w, r, http.StatusOK,
		map[string]any{"platform": a.exports.Platform(), "state": a.exports.State()},
		notice, platform)
}

func (a *app) waitForExportDisconnect(ctx context.Context) error {
	var err error
	for attempt := 0; attempt < 50; attempt++ {
		if err = a.exports.Disconnect(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return err
}

func (a *app) safePiDisconnect() {
	if a.pi == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = a.pi.Action(ctx, "detach")
	_ = a.pi.Action(ctx, "disconnect")
}

func (a *app) piPowerAction(w http.ResponseWriter, r *http.Request) {
	if a.pi == nil {
		http.Error(w, "Pi controls require automatic switching configuration",
			http.StatusServiceUnavailable)
		return
	}
	action := r.PathValue("action")
	helperAction, label := "", ""
	switch action {
	case "reboot":
		helperAction, label = "reboot", "reboot"
	case "shutdown":
		helperAction, label = "poweroff", "shut down"
	default:
		http.Error(w, "unsupported Pi power action", http.StatusBadRequest)
		return
	}
	if r.FormValue("confirm") != action {
		http.Error(w, "explicit Pi power confirmation is required", http.StatusBadRequest)
		return
	}
	a.switchMu.Lock()
	defer a.switchMu.Unlock()
	if err := a.pi.Action(r.Context(), helperAction); err != nil {
		http.Error(w, "Pi "+label+" failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	respondAction(w, r, http.StatusOK, map[string]string{"status": action},
		"Pi "+label+" requested. USB and NBD were disconnected first.", a.exports.Platform())
}

func (a *app) restoreGameCubeSave(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	manifest := a.activeGC
	a.mu.RUnlock()
	if manifest == nil || a.exports.Platform() != "gamecube" {
		http.Error(w, "select the matching GameCube export before restoring its save",
			http.StatusConflict)
		return
	}
	if err := a.exports.Disconnect(); err != nil {
		http.Error(w, "detach USB and disconnect Pi NBD before restore: "+err.Error(),
			http.StatusConflict)
		return
	}
	backupPath := filepath.Clean(r.FormValue("backup"))
	available, err := gamecube.ListSaveBackups(
		filepath.Join(a.dataDir, "gamecube", "save-backups"),
		manifest.Game.ID, manifest.Game.Revision)
	if err != nil {
		http.Error(w, "cannot list save backups", http.StatusInternalServerError)
		return
	}
	allowed := false
	for _, backup := range available {
		if backup.Path == backupPath {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "save backup is not in the managed backup set", http.StatusBadRequest)
		return
	}
	if _, err = gamecube.BackupMemoryCards(*manifest,
		filepath.Join(a.dataDir, "gamecube", "save-backups"),
		gamecube.DefaultSaveBackupRetention+1); err != nil {
		http.Error(w, "current save backup failed; restore refused: "+err.Error(),
			http.StatusConflict)
		return
	}
	if err = gamecube.RestoreMemoryCard(*manifest, backupPath, r.FormValue("name")); err != nil {
		http.Error(w, "save restore failed: "+err.Error(), http.StatusConflict)
		return
	}
	respondAction(w, r, http.StatusOK,
		map[string]string{"status": "restored", "state": string(a.exports.State())},
		"Latest validated memory-card backup restored.", "gamecube")
}

func (a *app) persistSnapshot() error {
	a.mu.RLock()
	data, err := json.MarshalIndent(a.disk.Snapshot(), "", "  ")
	a.mu.RUnlock()
	if err != nil {
		return err
	}
	path := filepath.Join(a.dataDir, "snapshots", a.disk.Snapshot().SnapshotID+".json")
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	return a.store.Publish(a.disk.Snapshot())
}

func (a *app) metrics(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "wiibridge_catalog_games %d\nwiibridge_scan_rejections %d\n", len(a.scan.Games), len(a.scan.Rejected))
}

func (a *app) dashboard(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	snapshot := a.disk.Snapshot().SnapshotID
	wiiGames := append([]model.Game(nil), a.scan.Games...)
	gameCubeGames := append([]gamecube.Game(nil), a.gcScan.Games...)
	wiiRejected := len(a.scan.Rejected)
	gameCubeRejected := len(a.gcScan.Rejected)
	wiiRejections := append([]scanner.Rejection(nil), a.scan.Rejected...)
	gameCubeRejections := append([]gamecube.Rejection(nil), a.gcScan.Rejected...)
	libraryRoot := a.root
	imports := make(map[string]importJob, len(a.imports))
	for key, job := range a.imports {
		imports[key] = job
	}
	csrf := a.csrf
	a.mu.RUnlock()
	totalWii, totalGameCube := len(wiiGames), len(gameCubeGames)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query != "" {
		matches := func(title, id string) bool {
			return strings.Contains(strings.ToLower(title), strings.ToLower(query)) ||
				strings.Contains(strings.ToLower(id), strings.ToLower(query))
		}
		filteredWii := wiiGames[:0]
		for _, game := range wiiGames {
			if matches(game.Title, game.ID) {
				filteredWii = append(filteredWii, game)
			}
		}
		wiiGames = filteredWii
		filteredGameCube := gameCubeGames[:0]
		for _, game := range gameCubeGames {
			if matches(game.Title, game.ID) {
				filteredGameCube = append(filteredGameCube, game)
			}
		}
		gameCubeGames = filteredGameCube
	}
	type gameCubeRow struct {
		gamecube.Game
		Backups  []gamecube.SaveBackup
		Settings gamecube.Settings
	}
	type reviewRow struct {
		Platform string
		Path     string
		Reason   string
	}
	displayPath := func(path string) string {
		relative, err := filepath.Rel(libraryRoot, path)
		if err == nil && relative != "." && relative != ".." &&
			!strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return relative
		}
		return path
	}
	review := make([]reviewRow, 0, wiiRejected+gameCubeRejected)
	for _, rejected := range wiiRejections {
		review = append(review, reviewRow{
			Platform: "Wii", Path: displayPath(rejected.Path), Reason: rejected.Reason,
		})
	}
	for _, rejected := range gameCubeRejections {
		review = append(review, reviewRow{
			Platform: "GameCube", Path: displayPath(rejected.Path), Reason: rejected.Reason,
		})
	}
	sort.Slice(review, func(i, j int) bool {
		if review[i].Platform == review[j].Platform {
			return review[i].Path < review[j].Path
		}
		return review[i].Platform < review[j].Platform
	})
	rows := make([]gameCubeRow, 0, len(gameCubeGames))
	for _, game := range gameCubeGames {
		backups, err := gamecube.ListSaveBackups(
			filepath.Join(a.dataDir, "gamecube", "save-backups"), game.ID, game.Revision)
		if err != nil {
			slog.Warn("cannot list GameCube save backups", "game_id", game.ID, "error", err)
		}
		settings, err := gamecube.LoadSettings(
			filepath.Join(a.dataDir, "gamecube"), game.ID, game.Revision)
		if err != nil {
			slog.Warn("cannot load GameCube settings", "game_id", game.ID, "error", err)
			settings = gamecube.DefaultSettings(game.ID, game.Revision)
		}
		rows = append(rows, gameCubeRow{Game: game, Backups: backups, Settings: settings})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	filter := r.URL.Query().Get("platform")
	if filter != "wii" && filter != "gamecube" {
		filter = "all"
	}
	importActive, importReady, importFailed := 0, 0, 0
	for _, job := range imports {
		switch job.Status {
		case "queued", "building":
			importActive++
		case "ready":
			importReady++
		case "failed":
			importFailed++
		}
	}
	piAddress := ""
	if a.pi != nil {
		piAddress = a.pi.Address()
	}
	data := map[string]any{
		"Version": version, "Snapshot": snapshot,
		"Wii": wiiGames, "GameCube": rows, "Filter": filter,
		"CSRF": csrf, "Platform": a.exports.Platform(), "State": a.exports.State(),
		"Query": query, "Notice": strings.TrimSpace(r.URL.Query().Get("notice")),
		"TotalWii": totalWii, "TotalGameCube": totalGameCube,
		"Rejected":        wiiRejected + gameCubeRejected,
		"WiiRejected":     wiiRejected,
		"GCRejected":      gameCubeRejected,
		"Review":          review,
		"AutomaticSwitch": a.pi != nil,
		"PiConnected":     false,
		"PiStatus":        bridgecontrol.Status{},
		"PiAddress":       piAddress,
		"ImportActive":    importActive, "ImportReady": importReady, "ImportFailed": importFailed,
	}
	if err := hostDashboard.Execute(w, data); err != nil {
		slog.Warn("dashboard render failed", "error", err)
	}
}

func humanBytes(size int64) string {
	const unit = int64(1024)
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exponent := unit, 0
	for value := size / unit; value >= unit && exponent < 4; value /= unit {
		div *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exponent])
}

var hostDashboard = template.Must(template.New("host").Funcs(template.FuncMap{
	"bytes": humanBytes,
	"time": func(value time.Time) string {
		return value.UTC().Format("2006-01-02 15:04 UTC")
	},
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>WiiBridge Host</title>
<style>
:root{color-scheme:dark;--bg:#080b12;--panel:#111724;--panel2:#161e2e;--line:#273349;--text:#f4f7fb;--muted:#9eabc0;--blue:#56a8ff;--cyan:#55e2d5;--green:#71e39a;--amber:#ffc766;--red:#ff7b87;--shadow:0 20px 50px #0007}
*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at 85% 0,#12315c66,transparent 32rem),var(--bg);color:var(--text);font:15px/1.5 Inter,ui-sans-serif,system-ui,-apple-system,sans-serif}a{color:inherit}.shell{max-width:1240px;margin:auto;padding:28px 22px 64px}.topbar{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-bottom:28px}.brand{display:flex;align-items:center;gap:13px}.mark{display:grid;place-items:center;width:44px;height:44px;border:1px solid #70b8ff66;border-radius:14px;background:linear-gradient(145deg,#173861,#0d1c31);box-shadow:inset 0 1px #ffffff24,0 8px 30px #2d83dc33;font-weight:850;color:#8dc9ff}.brand h1{font-size:20px;line-height:1.1;margin:0}.brand p,.muted{color:var(--muted);margin:3px 0 0}.status{display:flex;align-items:center;gap:10px;padding:9px 13px;border:1px solid var(--line);border-radius:999px;background:#0d1320}.dot{width:9px;height:9px;border-radius:50%;background:var(--green);box-shadow:0 0 14px var(--green)}.dot.offline{background:var(--red);box-shadow:0 0 14px var(--red)}.hero{display:grid;grid-template-columns:minmax(0,1.5fr) minmax(280px,.7fr);gap:18px;margin-bottom:18px}.card{border:1px solid var(--line);border-radius:20px;background:linear-gradient(150deg,#151d2bfa,#0d121dfa);box-shadow:var(--shadow)}.hero-main{padding:28px}.eyebrow{color:var(--cyan);font-size:12px;font-weight:800;letter-spacing:.13em;text-transform:uppercase}.hero h2{font-size:clamp(27px,4vw,44px);line-height:1.05;letter-spacing:-.04em;margin:12px 0}.hero p{max-width:680px;color:var(--muted);font-size:16px}.snapshot{padding:24px}.snapshot code{display:block;overflow:hidden;text-overflow:ellipsis;color:#b9c6da;font-size:12px}.metrics{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin:18px 0}.metric{display:block;padding:18px;text-decoration:none}.metric strong{display:block;font-size:28px;letter-spacing:-.04em}.metric span{color:var(--muted);font-size:13px}.metric[href]:hover{border-color:#496789}.pi-live{padding:20px;margin:18px 0}.pi-head{display:flex;align-items:center;justify-content:space-between;gap:12px}.pi-head h2{margin:0}.pi-address{display:flex;gap:8px;align-items:end;margin-top:14px}.pi-address label{flex:1;color:var(--muted);font-size:12px}.pi-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;margin-top:15px}.pi-value{padding:12px;border:1px solid var(--line);border-radius:12px;background:#0b111b}.pi-value small{display:block;color:var(--muted)}.pi-value strong{display:block;overflow-wrap:anywhere}.toolbar{display:flex;align-items:center;justify-content:space-between;gap:15px;margin:24px 0 16px}.tabs{display:flex;gap:6px;padding:5px;border:1px solid var(--line);border-radius:14px;background:#0d131e}.tab{padding:8px 14px;border-radius:10px;text-decoration:none;color:var(--muted);font-weight:700}.tab.active,.tab:hover{background:#1d2a3d;color:var(--text)}.search{display:flex;gap:8px;min-width:min(100%,350px)}input,select{width:100%;border:1px solid var(--line);border-radius:10px;background:#0a101a;color:var(--text);padding:10px 12px}button,.button{appearance:none;border:1px solid #3a7fc5;border-radius:10px;background:linear-gradient(#2786db,#1762a5);color:white;padding:9px 13px;font-weight:750;cursor:pointer;text-decoration:none}button:hover{filter:brightness(1.12)}button.secondary{border-color:var(--line);background:#172131}.danger{border-color:#a74450!important;background:#5d2028!important}.notice{margin:16px 0;padding:13px 16px;border:1px solid #347f7566;border-radius:13px;background:#14332e;color:#9ff4dc}.section-head{display:flex;justify-content:space-between;align-items:end;margin:28px 0 12px}.section-head h2{margin:0;font-size:22px}.section-head p{margin:0;color:var(--muted)}.table-wrap{overflow:auto}.library{width:100%;border-collapse:collapse}.library th{color:#91a2ba;font-size:11px;text-transform:uppercase;letter-spacing:.1em}.library th,.library td{text-align:left;padding:14px 16px;border-bottom:1px solid var(--line)}.library tr:last-child td{border:0}.title{font-weight:750}.tag{display:inline-flex;padding:3px 8px;border:1px solid var(--line);border-radius:999px;color:#b8c7da;background:#111a29;font-size:12px}.games{display:grid;grid-template-columns:repeat(auto-fit,minmax(310px,1fr));gap:14px}.game{padding:20px}.game-top{display:flex;justify-content:space-between;gap:12px}.game h3{margin:5px 0 2px;font-size:19px}.metadata{display:flex;flex-wrap:wrap;gap:7px;margin:14px 0}.health{padding:11px 12px;border-radius:11px;background:#0b111b;color:var(--muted)}.actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:14px}.actions form{margin:0}details{margin-top:14px;border-top:1px solid var(--line);padding-top:12px}summary{cursor:pointer;color:#b9c9dc;font-weight:700}.settings{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px;margin-top:12px}.settings label{color:var(--muted);font-size:12px}.settings button{grid-column:1/-1}.empty{padding:28px;text-align:center;color:var(--muted)}.review{margin-top:24px;padding:0 20px 20px}.review>summary{padding:20px 0;font-size:17px}.review-help{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:14px}.review-help p{margin:0;color:var(--muted)}.reason{color:#ffbec4}.path{word-break:break-all}.switcher{display:flex;align-items:center;justify-content:space-between;gap:18px;padding:20px;margin-top:24px}.switcher h2{margin:0;font-size:18px}.switcher p{margin:4px 0 0;color:var(--muted)}.power{padding:0 20px 20px;margin-top:14px}.power form{margin-top:12px}.confirm{display:flex;align-items:center;gap:8px;color:var(--muted)}.confirm input{width:auto}footer{margin-top:30px;color:#718096;font-size:12px}
@media(max-width:760px){.hero{grid-template-columns:1fr}.metrics,.pi-grid{grid-template-columns:repeat(2,1fr)}.toolbar,.topbar,.switcher,.pi-head{align-items:stretch;flex-direction:column}.search{min-width:0}.tabs{overflow:auto}.library th:nth-child(1),.library td:nth-child(1){display:none}.shell{padding:18px 14px 45px}}
</style></head><body><main class="shell">
<header class="topbar"><div class="brand"><div class="mark" aria-hidden="true">WB</div><div><h1>WiiBridge Host</h1><p>Private game-library bridge</p></div></div><div class="status"><span class="dot"></span><span><strong>{{.Platform}}</strong> · {{.State}}</span></div></header>
{{if .Notice}}<div class="notice" role="status">{{.Notice}}</div>{{end}}
<section class="hero"><div class="card hero-main"><span class="eyebrow">Direct Pi-to-Wii storage</span><h2>Your library, ready at the console.</h2><p>Browse validated Wii and GameCube backups, prepare Nintendont exports, and control the active read-only storage profile without changing source media.</p></div><aside class="card snapshot"><span class="eyebrow">Published snapshot</span><h3>Catalog is synchronized</h3><code title="{{.Snapshot}}">{{.Snapshot}}</code><p class="muted">Host version {{.Version}}</p></aside></section>
<section class="card pi-live" id=pi-live data-status-url=/api/v1/pi/status><div class=pi-head><div><span class=eyebrow>Live Raspberry Pi connection</span><h2><span class="dot offline" id=pi-dot></span> <span id=pi-connection>{{if .AutomaticSwitch}}Checking{{else}}Not configured{{end}}</span></h2></div><span class=tag id=pi-updated>{{if .AutomaticSwitch}}Waiting for cached status{{else}}Host credentials required{{end}}</span></div><form class=pi-address method=post action=/api/v1/pi/address><input type=hidden name=csrf value="{{.CSRF}}"><label>Raspberry Pi IP address<input name=address value="{{.PiAddress}}" inputmode=decimal placeholder="192.0.2.10" required {{if not .AutomaticSwitch}}disabled{{end}}></label><button class=secondary {{if not .AutomaticSwitch}}disabled{{end}}>Save address</button></form>{{if .AutomaticSwitch}}<p class=muted>Port 9443 and the pinned Pi certificate remain fixed. Status is collected once every 10 seconds regardless of how many dashboards are open.</p>{{else}}<p class=muted>Set <code>WIIBRIDGE_PI_ADMIN_TOKEN</code> and <code>WIIBRIDGE_PI_CERT</code> in the Host YAML, then restart the Host app to enable this IP control. Port 9443 and certificate pinning remain fixed.</p>{{end}}<div class=pi-grid><div class=pi-value><small>Controller state</small><strong id=pi-state>unknown</strong></div><div class=pi-value><small>Export mode</small><strong id=pi-export>unknown</strong></div><div class=pi-value><small>NBD / USB</small><strong id=pi-storage>unavailable</strong></div><div class=pi-value><small>USB controller</small><strong id=pi-usb>unavailable</strong></div><div class=pi-value><small>Board</small><strong id=pi-board>unavailable</strong></div><div class=pi-value><small>Network addresses</small><strong id=pi-addresses>unavailable</strong></div><div class=pi-value><small>Provisioning</small><strong id=pi-provision>unknown</strong></div><div class=pi-value><small>Automatic attach</small><strong id=pi-attach>unknown</strong></div></div></section><script src=/assets/pi-status.js defer></script>
<section class="metrics" aria-label="Library summary"><div class="card metric"><strong>{{.TotalWii}}</strong><span>Wii titles</span></div><div class="card metric"><strong>{{.TotalGameCube}}</strong><span>GameCube titles</span></div><div class="card metric"><strong>{{.ImportReady}}</strong><span>Ready imports</span></div><a class="card metric" href="#library-review"><strong>{{.Rejected}}</strong><span>Needs review · open details</span></a></section>
<div class="toolbar"><nav class="tabs" aria-label="Platform filter"><a class="tab {{if eq .Filter "all"}}active{{end}}" href="?platform=all{{if .Query}}&q={{.Query}}{{end}}">All</a><a class="tab {{if eq .Filter "wii"}}active{{end}}" href="?platform=wii{{if .Query}}&q={{.Query}}{{end}}">Wii</a><a class="tab {{if eq .Filter "gamecube"}}active{{end}}" href="?platform=gamecube{{if .Query}}&q={{.Query}}{{end}}">GameCube</a></nav><form class="search" method=get><input type=hidden name=platform value="{{.Filter}}"><input name=q value="{{.Query}}" placeholder="Search title or game ID" aria-label="Search title or game ID"><button type=submit>Search</button>{{if .Query}}<a class="button secondary" href="?platform={{.Filter}}">Clear</a>{{end}}</form></div>
{{if ne .Filter "gamecube"}}<section><div class="section-head"><div><h2>Wii library</h2><p>Wii remains the safe default export.</p></div><span class="tag">{{len .Wii}} shown</span></div><div class="card table-wrap">{{if .Wii}}<table class=library><thead><tr><th>Platform</th><th>Title</th><th>Game ID</th><th>Virtual size</th></tr></thead><tbody>{{range .Wii}}<tr><td><span class=tag>Wii</span></td><td class=title>{{.Title}}</td><td><code>{{.ID}}</code></td><td>{{bytes .Size}}</td></tr>{{end}}</tbody></table>{{else}}<div class=empty>No Wii titles match this view.</div>{{end}}</div></section>{{end}}
{{if ne .Filter "wii"}}<section><div class="section-head"><div><h2>GameCube library</h2><p>{{if .ImportActive}}{{.ImportActive}} import in progress · {{end}}{{if .ImportFailed}}{{.ImportFailed}} failed import{{else}}Nintendont-compatible exports{{end}}</p></div><span class=tag>{{len .GameCube}} shown</span></div><div class=games>{{range .GameCube}}<article class="card game"><div class=game-top><div><span class=eyebrow>GameCube</span><h3>{{.Title}}</h3><code>{{.ID}} · revision {{.Revision}}</code></div><span class=tag>{{.Validation}}</span></div><div class=metadata><span class=tag>{{.Region}}</span><span class=tag>{{.Format}}</span><span class=tag>{{.DiscCount}} disc{{if ne .DiscCount 1}}s{{end}}</span></div><div class=health>{{if .Backups}}<strong>{{len .Backups}} validated save backups</strong><br>Latest {{time (index .Backups 0).Created}}{{else}}No emulated-memory-card backup recorded yet.{{end}}</div><div class=actions>
<form method=post action=/api/v1/gamecube/import><input type=hidden name=csrf value="{{$.CSRF}}"><input type=hidden name=id value="{{.ID}}"><input type=hidden name=revision value="{{.Revision}}"><button>Prepare export</button></form>
<form method=post action=/api/v1/export/gamecube><input type=hidden name=csrf value="{{$.CSRF}}"><input type=hidden name=id value="{{.ID}}"><input type=hidden name=revision value="{{.Revision}}"><button class=secondary>Select</button></form>
{{if .Backups}}{{$latest := index .Backups 0}}<form method=post action=/api/v1/gamecube/saves/restore><input type=hidden name=csrf value="{{$.CSRF}}"><input type=hidden name=backup value="{{$latest.Path}}"><input type=hidden name=name value="{{$latest.Name}}"><button class=secondary>Restore save</button></form>{{end}}</div>
<details><summary>Compatibility settings</summary><form class=settings method=post action=/api/v1/gamecube/settings><input type=hidden name=csrf value="{{$.CSRF}}"><input type=hidden name=id value="{{.ID}}"><input type=hidden name=revision value="{{.Revision}}">
<label>Memory card<select name=memory_card><option value=physical {{if eq .Settings.MemoryCard "physical"}}selected{{end}}>Physical</option><option value=emulated {{if eq .Settings.MemoryCard "emulated"}}selected{{end}}>Emulated</option></select></label>
<label>Card size<select name=memory_card_size><option value=0 {{if eq .Settings.MemoryCardSize 0}}selected{{end}}>59 blocks</option><option value=1 {{if eq .Settings.MemoryCardSize 1}}selected{{end}}>123 blocks</option><option value=2 {{if eq .Settings.MemoryCardSize 2}}selected{{end}}>251 blocks</option><option value=3 {{if eq .Settings.MemoryCardSize 3}}selected{{end}}>507 blocks</option><option value=4 {{if eq .Settings.MemoryCardSize 4}}selected{{end}}>1019 blocks</option><option value=5 {{if eq .Settings.MemoryCardSize 5}}selected{{end}}>2043 blocks</option></select></label>
<label>Video mode<select name=video_mode><option {{if eq .Settings.VideoMode "auto"}}selected{{end}}>auto</option><option {{if eq .Settings.VideoMode "disc"}}selected{{end}}>disc</option><option {{if eq .Settings.VideoMode "ntsc"}}selected{{end}}>ntsc</option><option {{if eq .Settings.VideoMode "pal50"}}selected{{end}}>pal50</option><option {{if eq .Settings.VideoMode "pal60"}}selected{{end}}>pal60</option><option {{if eq .Settings.VideoMode "mpal"}}selected{{end}}>mpal</option></select></label>
<label>Controller<select name=controller_mode><option {{if eq .Settings.ControllerMode "auto"}}selected{{end}}>auto</option><option {{if eq .Settings.ControllerMode "native"}}selected{{end}}>native</option><option {{if eq .Settings.ControllerMode "hid"}}selected{{end}}>hid</option></select></label>
<label>Disc speed<select name=disc_speed><option {{if eq .Settings.DiscSpeed "auto"}}selected{{end}}>auto</option><option {{if eq .Settings.DiscSpeed "original"}}selected{{end}}>original</option></select></label><input type=hidden name=return_to_loader value=1><button>Save settings</button></form></details></article>{{else}}<div class="card empty">No GameCube titles match this view.</div>{{end}}</div></section>{{end}}
<details class="card review" id="library-review" {{if .Rejected}}open{{end}}><summary>Library review — {{.Rejected}} item{{if ne .Rejected 1}}s{{end}}</summary><div class=review-help><p>{{.WiiRejected}} Wii · {{.GCRejected}} GameCube. Rejected files are never modified or exported.</p><form method=post action=/api/v1/scan><input type=hidden name=csrf value="{{.CSRF}}"><button class=secondary>Rescan library</button></form></div>{{if .Review}}<div class=table-wrap><table class=library><thead><tr><th>Scanner</th><th>Source path</th><th>Reason</th></tr></thead><tbody>{{range .Review}}<tr><td><span class=tag>{{.Platform}}</span></td><td class=path><code>{{.Path}}</code></td><td class=reason>{{.Reason}}</td></tr>{{end}}</tbody></table></div>{{else}}<div class=empty>No rejected library entries.</div>{{end}}</details>
<section class="card switcher"><div><h2>Platform switching</h2>{{if .AutomaticSwitch}}<p>Automatic safety sequence enabled: detach USB → disconnect NBD → switch export → reconnect → attach.</p>{{else}}<p>Manual mode: detach USB and disconnect Pi NBD before switching the host export.</p>{{end}}</div><form method=post action=/api/v1/export/wii><input type=hidden name=csrf value="{{.CSRF}}"><button>Use Wii export</button></form></section>
{{if .AutomaticSwitch}}<details class="card power"><summary>Pi power controls</summary><p class=muted>Both actions safely detach USB and disconnect NBD first.</p><form method=post action=/api/v1/pi/reboot><input type=hidden name=csrf value="{{.CSRF}}"><label class=confirm><input type=checkbox name=confirm value=reboot required> Confirm Raspberry Pi reboot</label><button class=secondary>Reboot Pi</button></form><form method=post action=/api/v1/pi/shutdown><input type=hidden name=csrf value="{{.CSRF}}"><label class=confirm><input type=checkbox name=confirm value=shutdown required> Confirm Raspberry Pi shutdown</label><button class=danger>Shut down Pi</button></form></details>{{end}}
<footer>WiiBridge keeps source libraries immutable. Hardware deployment and write-enabled GameCube saves remain explicit operator actions.</footer>
</main></body></html>`))

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func respondAction(w http.ResponseWriter, r *http.Request, status int, value any, notice, platform string) {
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		query := url.Values{"notice": {notice}, "platform": {platform}}
		http.Redirect(w, r, "/?"+query.Encode(), http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
