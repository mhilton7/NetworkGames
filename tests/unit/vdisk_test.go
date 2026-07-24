package unit_test

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"

	"networkgames/server/host-daemon/scanner"
	"networkgames/server/host-daemon/vdisk"
	"networkgames/shared/model"
	"networkgames/tests/testutil"
)

func TestVirtualDiskDeterminismAndBounds(t *testing.T) {
	root := t.TempDir()
	if err := testutil.SyntheticWBFS(filepath.Join(root, "game.wbfs"), "TEST01", "Synthetic", 3<<20); err != nil {
		t.Fatal(err)
	}
	result, err := scanner.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	a, err := vdisk.Build("all", result.Games, "test")
	if err != nil {
		t.Fatal(err)
	}
	b, err := vdisk.Build("all", result.Games, "test")
	if err != nil {
		t.Fatal(err)
	}
	if a.Snapshot().MetadataHash != b.Snapshot().MetadataHash ||
		!bytes.Equal(a.MetadataBytes(), b.MetadataBytes()) {
		t.Fatal("metadata is not deterministic")
	}
	mbr := make([]byte, 512)
	if _, err = a.ReadAt(mbr, 0); err != nil {
		t.Fatal(err)
	}
	if mbr[510] != 0x55 || mbr[511] != 0xaa ||
		binary.LittleEndian.Uint32(mbr[454:458]) != 2048 {
		t.Fatal("invalid MBR")
	}
	if _, err = a.ReadAt(make([]byte, 513), a.Size()-512); err == nil {
		t.Fatal("out-of-range read accepted")
	}
}

func TestVirtualLargeFileSplitting(t *testing.T) {
	const firstSegment = int64(0xfffff000)
	game := model.Game{ID: "SPLT01", Size: firstSegment + 8192,
		Sources: []model.Source{{Path: "/synthetic/not-opened", Length: firstSegment + 8192}}}
	disk, err := vdisk.Build("large-file", []model.Game{game}, "test")
	if err != nil {
		t.Fatal(err)
	}
	files := disk.Snapshot().VirtualFiles
	if len(files) != 2 ||
		files[0].Path != "/wbfs/SPLT01.wbfs" ||
		files[0].Length != firstSegment ||
		files[1].Path != "/wbfs/SPLT01.wbf1" ||
		files[1].LogicalStart != firstSegment ||
		files[1].Length != 8192 {
		t.Fatalf("unexpected virtual split map: %#v", files)
	}
}
