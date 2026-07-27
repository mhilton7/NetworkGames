package unit_test

import (
	"os"
	"path/filepath"
	"testing"

	"wiibridge/server/host-daemon/scanner"
	"wiibridge/tests/testutil"
)

func TestScannerValidAndMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Synthetic [ABCD01].wbfs")
	if err := testutil.SyntheticWBFS(path, "ABCD01", "Synthetic Test", 2<<20); err != nil {
		t.Fatal(err)
	}
	result, err := scanner.Scan(root)
	if err != nil || len(result.Games) != 1 || result.Games[0].ID != "ABCD01" {
		t.Fatalf("scan = %#v, %v", result, err)
	}
	source := result.Games[0].Sources[0]
	if err := scanner.VerifySource(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, source.Size+1); err != nil {
		t.Fatal(err)
	}
	if err := scanner.VerifySource(source); err == nil {
		t.Fatal("mutation not detected")
	}
}

func TestScannerRejectsSymlinkAndBadMagic(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bad.wbfs"), []byte("not wbfs"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "escape.wbfs")); err != nil {
		t.Fatal(err)
	}
	result, err := scanner.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 0 || len(result.Rejected) != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
}
