// networkgames-pi-controller exposes local status and delegates the small set
// of privileged state transitions to networkgames-helper.
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type status struct {
	Target       string `json:"target"`
	Board        string `json:"detected_board"`
	BoardOK      bool   `json:"board_compatible"`
	Provisioned  bool   `json:"provisioned"`
	NBDConnected bool   `json:"nbd_connected"`
	USBAttached  bool   `json:"usb_attached"`
	State        string `json:"state"`
}

func main() {
	token, err := os.ReadFile("/etc/networkgames/admin.token")
	if err != nil || len(strings.TrimSpace(string(token))) < 20 {
		log.Fatal("unique admin token has not been provisioned")
	}
	expected, err := os.ReadFile("/usr/share/networkgames/board-target")
	if err != nil {
		log.Fatal(err)
	}
	authToken := strings.TrimSpace(string(token))
	target := strings.TrimSpace(string(expected))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, collect(target))
	})
	mux.HandleFunc("GET /api/v1/status", authenticated(authToken, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, collect(target))
	}))
	mux.HandleFunc("GET /", authenticated(authToken, func(w http.ResponseWriter, _ *http.Request) {
		s := collect(target)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>NetworkGames Bridge</title>"+
			"<h1>NetworkGames Bridge</h1><dl><dt>Target</dt><dd>%s</dd>"+
			"<dt>Detected board</dt><dd>%s</dd><dt>State</dt><dd>%s</dd>"+
			"<dt>NBD connected</dt><dd>%t</dd><dt>USB attached</dt><dd>%t</dd></dl>",
			s.Target, s.Board, s.State, s.NBDConnected, s.USBAttached)
	}))
	mux.HandleFunc("POST /api/v1/action/{action}", authenticated(authToken, func(w http.ResponseWriter, r *http.Request) {
		action := r.PathValue("action")
		switch action {
		case "connect", "disconnect", "attach", "detach", "clear-cache", "test":
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, "/usr/bin/sudo", "-n",
			"/usr/libexec/networkgames-helper", action).CombinedOutput()
		if err != nil {
			http.Error(w, "action failed: "+strings.TrimSpace(string(output)), http.StatusConflict)
			return
		}
		writeJSON(w, collect(target))
	}))
	server := &http.Server{
		Addr: ":9443", Handler: headers(mux), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second,
	}
	log.Fatal(server.ListenAndServeTLS("/etc/networkgames/device.crt", "/etc/networkgames/device.key"))
}

func authenticated(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if user, password, ok := r.BasicAuth(); ok && user == "admin" {
			got = password
		}
		if len(got) != len(token) || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="NetworkGames Bridge", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func collect(target string) status {
	boardBytes, _ := os.ReadFile("/proc/device-tree/model")
	board := strings.TrimRight(string(boardBytes), "\x00\n")
	ok := boardCompatible(target, board)
	state := "ready"
	if !ok {
		state = "wrong-board-recovery"
	}
	return status{
		Target: target, Board: board, BoardOK: ok,
		Provisioned:  exists("/etc/networkgames/provisioned"),
		NBDConnected: exists("/run/networkgames/nbd-connected"),
		USBAttached:  exists("/run/networkgames/usb-attached"), State: state,
	}
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
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
