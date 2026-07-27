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
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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
	mux.HandleFunc("GET /api/v1/scan", a.auth(a.scanResult))
	mux.HandleFunc("POST /api/v1/scan", a.auth(a.rescan))
	mux.HandleFunc("GET /api/v1/gamecube", a.auth(a.gamecubeResult))
	mux.HandleFunc("GET /api/v1/gamecube/imports", a.auth(a.gamecubeImports))
	mux.HandleFunc("POST /api/v1/gamecube/import", a.auth(a.importGameCube))
	mux.HandleFunc("POST /api/v1/gamecube/settings", a.auth(a.saveGameCubeSettings))
	mux.HandleFunc("POST /api/v1/gamecube/saves/restore", a.auth(a.restoreGameCubeSave))
	mux.HandleFunc("POST /api/v1/export/{platform}", a.auth(a.selectExport))
	mux.HandleFunc("GET /metrics", a.auth(a.metrics))
	mux.HandleFunc("GET /", a.auth(a.dashboard))
	web := &http.Server{
		Addr: env("WIIBRIDGE_HTTPS_LISTEN", ":8445"), Handler: securityHeaders(mux),
		TLSConfig: tlsConfig.Clone(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second,
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
		"export_state": a.exports.State(), "uptime_seconds": int(time.Since(started).Seconds()),
	})
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

func (a *app) rescan(w http.ResponseWriter, _ *http.Request) {
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
	writeJSON(w, disk.Snapshot())
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"cache_key": key, "status": "queued"})
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
	writeJSON(w, settings)
}

func (a *app) selectExport(w http.ResponseWriter, r *http.Request) {
	platform := r.PathValue("platform")
	if platform != "wii" && platform != "gamecube" {
		http.Error(w, "unsupported platform", http.StatusBadRequest)
		return
	}
	var next exportprofile.Profile = a.wii
	var selectedManifest *gamecube.VolumeManifest
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
		key := gamecube.CacheKey(game, settings.MemoryCard)
		manifestPath := filepath.Join(a.dataDir, "gamecube", "cache", "ready", key, "manifest.json")
		manifest, err := gamecube.LoadAndValidateVolume(manifestPath)
		if err != nil {
			http.Error(w, "GameCube cache is not ready: "+err.Error(), http.StatusConflict)
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
	}
	a.mu.RLock()
	previousManifest := a.activeGC
	a.mu.RUnlock()
	if err := a.exports.Disconnect(); err != nil {
		if next != a.wii {
			_ = next.Close()
		}
		http.Error(w, "disconnect Pi NBD session before switching: "+err.Error(), http.StatusConflict)
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
	writeJSON(w, map[string]any{"platform": a.exports.Platform(), "state": a.exports.State()})
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
	writeJSON(w, map[string]string{"status": "restored", "state": string(a.exports.State())})
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
	csrf := a.csrf
	a.mu.RUnlock()
	type gameCubeRow struct {
		gamecube.Game
		Backups  []gamecube.SaveBackup
		Settings gamecube.Settings
	}
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
	data := map[string]any{
		"Version": version, "Snapshot": snapshot,
		"Wii": wiiGames, "GameCube": rows, "Filter": filter,
		"CSRF": csrf, "Platform": a.exports.Platform(), "State": a.exports.State(),
	}
	if err := hostDashboard.Execute(w, data); err != nil {
		slog.Warn("dashboard render failed", "error", err)
	}
}

var hostDashboard = template.Must(template.New("host").Parse(`<!doctype html>
<meta charset=utf-8><title>WiiBridge Host</title>
<style>body{font:16px system-ui;max-width:1100px;margin:2rem auto;padding:0 1rem}nav a{margin-right:1rem}table{border-collapse:collapse;width:100%}th,td{padding:.45rem;border-bottom:1px solid #ccc;text-align:left}form{display:inline}button,select{padding:.35rem}</style>
<h1>WiiBridge Host</h1>
<p>Version {{.Version}} · export <strong>{{.Platform}}</strong> ({{.State}})</p>
<nav><a href="?platform=all">All</a><a href="?platform=wii">Wii</a><a href="?platform=gamecube">GameCube</a></nav>
{{if ne .Filter "gamecube"}}<h2>Wii</h2><p>{{len .Wii}} titles. Wii remains the default export.</p>
<table><tr><th>Platform</th><th>Title</th><th>ID</th><th>Size</th></tr>
{{range .Wii}}<tr><td>Wii</td><td>{{.Title}}</td><td>{{.ID}}</td><td>{{.Size}}</td></tr>{{end}}</table>{{end}}
{{if ne .Filter "wii"}}<h2>GameCube</h2>
<table><tr><th>Platform</th><th>Title</th><th>ID</th><th>Region</th><th>Format</th><th>Discs</th><th>Status</th><th>Save health</th><th>Actions</th></tr>
{{range .GameCube}}<tr><td>GameCube</td><td>{{.Title}}</td><td>{{.ID}}</td><td>{{.Region}}</td><td>{{.Format}}</td><td>{{.DiscCount}}</td><td>{{.Validation}}</td><td>
{{if .Backups}}{{len .Backups}} validated backups; latest {{(index .Backups 0).Created}}{{else}}No validated backup yet{{end}}</td><td>
<form method=post action=/api/v1/gamecube/import><input type=hidden name=csrf value="{{$.CSRF}}"><input type=hidden name=id value="{{.ID}}"><input type=hidden name=revision value="{{.Revision}}"><button>Import</button></form>
<form method=post action=/api/v1/export/gamecube><input type=hidden name=csrf value="{{$.CSRF}}"><input type=hidden name=id value="{{.ID}}"><input type=hidden name=revision value="{{.Revision}}"><button>Select</button></form>
{{if .Backups}}{{$latest := index .Backups 0}}<form method=post action=/api/v1/gamecube/saves/restore><input type=hidden name=csrf value="{{$.CSRF}}"><input type=hidden name=backup value="{{$latest.Path}}"><input type=hidden name=name value="{{$latest.Name}}"><button>Restore latest</button></form>{{end}}
<details><summary>Per-game Nintendont settings</summary><form method=post action=/api/v1/gamecube/settings>
<input type=hidden name=csrf value="{{$.CSRF}}"><input type=hidden name=id value="{{.ID}}"><input type=hidden name=revision value="{{.Revision}}">
<label>Memory card <select name=memory_card><option value=physical {{if eq .Settings.MemoryCard "physical"}}selected{{end}}>Physical</option><option value=emulated {{if eq .Settings.MemoryCard "emulated"}}selected{{end}}>Emulated</option></select></label>
<label>Card size <select name=memory_card_size><option value=0 {{if eq .Settings.MemoryCardSize 0}}selected{{end}}>59 blocks</option><option value=1 {{if eq .Settings.MemoryCardSize 1}}selected{{end}}>123 blocks</option><option value=2 {{if eq .Settings.MemoryCardSize 2}}selected{{end}}>251 blocks</option><option value=3 {{if eq .Settings.MemoryCardSize 3}}selected{{end}}>507 blocks</option><option value=4 {{if eq .Settings.MemoryCardSize 4}}selected{{end}}>1019 blocks</option><option value=5 {{if eq .Settings.MemoryCardSize 5}}selected{{end}}>2043 blocks</option></select></label>
<label>Video <select name=video_mode><option {{if eq .Settings.VideoMode "auto"}}selected{{end}}>auto</option><option {{if eq .Settings.VideoMode "disc"}}selected{{end}}>disc</option><option {{if eq .Settings.VideoMode "ntsc"}}selected{{end}}>ntsc</option><option {{if eq .Settings.VideoMode "pal50"}}selected{{end}}>pal50</option><option {{if eq .Settings.VideoMode "pal60"}}selected{{end}}>pal60</option><option {{if eq .Settings.VideoMode "mpal"}}selected{{end}}>mpal</option></select></label>
<label>Controller <select name=controller_mode><option {{if eq .Settings.ControllerMode "auto"}}selected{{end}}>auto</option><option {{if eq .Settings.ControllerMode "native"}}selected{{end}}>native</option><option {{if eq .Settings.ControllerMode "hid"}}selected{{end}}>hid</option></select></label>
<label>Disc speed <select name=disc_speed><option {{if eq .Settings.DiscSpeed "auto"}}selected{{end}}>auto</option><option {{if eq .Settings.DiscSpeed "original"}}selected{{end}}>original</option></select></label>
<input type=hidden name=return_to_loader value=1><button>Save settings</button></form></details>
</td></tr>{{end}}</table>{{end}}
<h2>Platform switch</h2><p>Detach USB and disconnect Pi NBD before switching.</p>
<form method=post action=/api/v1/export/wii><input type=hidden name=csrf value="{{.CSRF}}"><button>Return to Wii mode</button></form>`))

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
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
