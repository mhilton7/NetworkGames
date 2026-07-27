package integration_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"wiibridge/server/host-daemon/scanner"
	"wiibridge/server/host-daemon/vdisk"
	"wiibridge/tests/testutil"
)

func TestSyntheticCatalog1000(t *testing.T) {
	if testing.Short() {
		t.Skip("large catalog test")
	}
	root := t.TempDir()
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("T%05d", i)
		if err := testutil.SyntheticWBFS(filepath.Join(root, id+".wbfs"), id, "Synthetic "+id, 2<<20); err != nil {
			t.Fatal(err)
		}
	}
	result, err := scanner.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 1000 || len(result.Rejected) != 0 {
		t.Fatalf("games=%d rejected=%d", len(result.Games), len(result.Rejected))
	}
	disk, err := vdisk.Build("all-1000", result.Games, "test")
	if err != nil {
		t.Fatal(err)
	}
	if disk.Size() < 2_000_000_000 {
		t.Fatalf("unexpected capacity: %d", disk.Size())
	}
}
