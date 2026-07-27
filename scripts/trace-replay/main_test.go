package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestReplayValidatesLengthContentAndBounds(t *testing.T) {
	payload := []byte("0123456789abcdef")
	sum := sha256.Sum256(payload[4:12])
	trace := `{"name":"known","offset":4,"length":8,"sha256":"` +
		hex.EncodeToString(sum[:]) + "\"}\n"
	var output bytes.Buffer
	if err := replay(bytes.NewReader(payload), strings.NewReader(trace), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"read_bytes":8`) ||
		!strings.Contains(output.String(), `"name":"known"`) {
		t.Fatalf("unexpected replay output: %s", output.String())
	}

	output.Reset()
	err := replay(bytes.NewReader(payload),
		strings.NewReader(`{"offset":12,"length":8}`+"\n"), &output)
	if err == nil || !strings.Contains(output.String(), `"error":"EOF"`) {
		t.Fatalf("out-of-range replay was not reported: err=%v output=%s", err, output.String())
	}
}

func TestReplayRejectsInvalidRequests(t *testing.T) {
	err := replay(bytes.NewReader(nil),
		strings.NewReader(`{"offset":-1,"length":512}`+"\n"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("invalid request accepted: %v", err)
	}
}
