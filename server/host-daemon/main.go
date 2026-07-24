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

	"networkgames/server/host-daemon/scanner"
	"networkgames/server/host-daemon/store"
	"networkgames/server/host-daemon/vdisk"
	"networkgames/server/nbd-plugin"
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
	authMu   sync.Mutex
	failures map[string]authFailure
}

type authFailure struct {
	count int
	reset time.Time
}

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
	url := fs.String("url", "https://127.0.0.1:8443/healthz", "health URL")
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
	root := env("NETWORKGAMES_LIBRARY", "/library")
	dataDir := env("NETWORKGAMES_DATA", "/data")
	token := os.Getenv("NETWORKGAMES_ADMIN_TOKEN")
	if len(token) < 20 {
		return errors.New("NETWORKGAMES_ADMIN_TOKEN must contain at least 20 characters")
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
	database, err := store.Open(filepath.Join(dataDir, "networkgames.sqlite3"))
	if err != nil {
		return err
	}
	defer database.Close()
	a := &app{root: root, dataDir: dataDir, disk: disk, scan: result,
		tokenSum: sha256.Sum256([]byte(token)), started: time.Now(), store: database,
		failures: make(map[string]authFailure)}
	if err := a.persistSnapshot(); err != nil {
		return err
	}
	tlsConfig, err := mutualTLSConfig(
		env("NETWORKGAMES_TLS_CERT", "/certs/server.crt"),
		env("NETWORKGAMES_TLS_KEY", "/certs/server.key"),
		env("NETWORKGAMES_TLS_CLIENT_CA", "/certs/clients-ca.crt"),
	)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	nbdListener, err := net.Listen("tcp", env("NETWORKGAMES_NBD_LISTEN", ":10809"))
	if err != nil {
		return err
	}
	nbdServer := &nbd.Server{
		BackendProvider: func() nbd.Backend {
			a.mu.RLock()
			defer a.mu.RUnlock()
			return a.disk
		},
		TLS: tlsConfig, ExportName: env("NETWORKGAMES_EXPORT", "all"),
		Deadline: 30 * time.Second, MaxRequest: 1 << 20,
	}
	errs := make(chan error, 2)
	go func() { errs <- nbdServer.Serve(ctx, nbdListener) }()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /api/v1/status", a.auth(a.status))
	mux.HandleFunc("GET /api/v1/scan", a.auth(a.scanResult))
	mux.HandleFunc("POST /api/v1/scan", a.auth(a.rescan))
	mux.HandleFunc("GET /metrics", a.auth(a.metrics))
	mux.HandleFunc("GET /", a.auth(a.dashboard))
	web := &http.Server{
		Addr: env("NETWORKGAMES_HTTPS_LISTEN", ":8443"), Handler: securityHeaders(mux),
		TLSConfig: tlsConfig.Clone(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10,
	}
	web.TLSConfig.ClientAuth = tls.NoClientCert
	web.TLSConfig.MinVersion = tls.VersionTLS13
	web.TLSConfig.MaxVersion = 0
	go func() {
		errs <- web.ListenAndServeTLS(
			env("NETWORKGAMES_TLS_CERT", "/certs/server.crt"),
			env("NETWORKGAMES_TLS_KEY", "/certs/server.key"),
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
	probe := filepath.Join(path, ".networkgames-write-probe-"+strconv.Itoa(os.Getpid()))
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
			w.Header().Set("WWW-Authenticate", `Basic realm="NetworkGames", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead &&
			r.Header.Get("X-NetworkGames-CSRF") != "1" {
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
	defer a.mu.RUnlock()
	writeJSON(w, map[string]any{
		"version": version, "snapshot": a.disk.Snapshot(), "games": len(a.scan.Games),
		"rejected": len(a.scan.Rejected), "uptime_seconds": int(time.Since(a.started).Seconds()),
	})
}

func (a *app) scanResult(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	writeJSON(w, a.scan)
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
	a.mu.Lock()
	a.scan, a.disk = result, disk
	a.mu.Unlock()
	if err := a.persistSnapshot(); err != nil {
		http.Error(w, "snapshot persistence failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, disk.Snapshot())
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
	fmt.Fprintf(w, "networkgames_catalog_games %d\nnetworkgames_scan_rejections %d\n", len(a.scan.Games), len(a.scan.Rejected))
}

func (a *app) dashboard(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>NetworkGames Host</title><h1>NetworkGames Host</h1><p>Version %s</p><p>Snapshot %s</p><p>%d games; %d rejected files</p>", version, hex.EncodeToString([]byte(a.disk.Snapshot().SnapshotID))[:16], len(a.scan.Games), len(a.scan.Rejected))
}

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
