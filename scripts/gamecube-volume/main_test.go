package main

import (
	"os"
	"path/filepath"
	"testing"

	"wiibridge/tests/testutil"
)

func TestScanCommandUsesSyntheticFixture(t *testing.T) {
	root := t.TempDir()
	if err := testutil.SyntheticGameCubeISO(
		filepath.Join(root, "game.iso"), "GCLI01", "CLI Test", 0, 0, 2<<20); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"scan", "-library", root}); err != nil {
		t.Fatal(err)
	}
}

func TestCommandsRejectMissingRequiredArguments(t *testing.T) {
	for _, command := range [][]string{{"scan"}, {"build"}, {"validate"}} {
		if err := run(command); err == nil {
			t.Fatalf("%s accepted missing arguments", command[0])
		}
	}
	if err := run([]string{"unknown"}); err == nil {
		t.Fatal("unknown command accepted")
	}
}

func TestValidateRejectsIncompleteManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"schema":1,"complete":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"validate", "-manifest", path}); err == nil {
		t.Fatal("incomplete manifest accepted")
	}
}
