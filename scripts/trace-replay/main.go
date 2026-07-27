// Command trace-replay replays positional reads against a read-only disk or
// image. It is intended for opt-in diagnostics; it never opens the target for
// writing and does not contain game data.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

type request struct {
	Name   string `json:"name,omitempty"`
	Offset int64  `json:"offset"`
	Length int    `json:"length"`
	SHA256 string `json:"sha256,omitempty"`
}

type result struct {
	Name      string `json:"name,omitempty"`
	Offset    int64  `json:"offset"`
	Length    int    `json:"length"`
	ReadBytes int    `json:"read_bytes"`
	LatencyUS int64  `json:"latency_us"`
	SHA256    string `json:"sha256"`
	Error     string `json:"error,omitempty"`
}

func replay(target io.ReaderAt, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	encoder := json.NewEncoder(output)
	line := 0
	var failures int
	for scanner.Scan() {
		line++
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			return fmt.Errorf("trace line %d: %w", line, err)
		}
		if req.Offset < 0 || req.Length <= 0 {
			return fmt.Errorf("trace line %d: offset must be non-negative and length positive", line)
		}
		buf := make([]byte, req.Length)
		start := time.Now()
		n, readErr := target.ReadAt(buf, req.Offset)
		elapsed := time.Since(start)
		sum := sha256.Sum256(buf[:n])
		got := hex.EncodeToString(sum[:])
		res := result{
			Name: req.Name, Offset: req.Offset, Length: req.Length,
			ReadBytes: n, LatencyUS: elapsed.Microseconds(), SHA256: got,
		}
		switch {
		case readErr != nil:
			res.Error = readErr.Error()
		case n != req.Length:
			res.Error = fmt.Sprintf("short read: got %d bytes, want %d", n, req.Length)
		case req.SHA256 != "" && req.SHA256 != got:
			res.Error = fmt.Sprintf("sha256 mismatch: got %s, want %s", got, req.SHA256)
		}
		if res.Error != "" {
			failures++
		}
		if err := encoder.Encode(res); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if failures != 0 {
		return fmt.Errorf("%d replay request(s) failed", failures)
	}
	return nil
}

func run(args []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("trace-replay", flag.ContinueOnError)
	flags.SetOutput(stderr)
	targetPath := flags.String("target", "", "read-only disk or image path")
	tracePath := flags.String("trace", "", "JSON-lines trace path (default: stdin)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *targetPath == "" {
		return errors.New("-target is required")
	}
	target, err := os.Open(*targetPath)
	if err != nil {
		return err
	}
	defer target.Close()
	var input io.Reader = os.Stdin
	if *tracePath != "" {
		trace, openErr := os.Open(*tracePath)
		if openErr != nil {
			return openErr
		}
		defer trace.Close()
		input = trace
	}
	return replay(target, input, os.Stdout)
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
