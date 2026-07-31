package unit_test

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"

	"wiibridge/server/host-daemon/scanner"
	"wiibridge/server/host-daemon/vdisk"
	"wiibridge/shared/model"
	"wiibridge/tests/testutil"
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
	diskSignature := binary.LittleEndian.Uint32(mbr[440:444])
	if diskSignature == 0 {
		t.Fatal("MBR disk signature is zero")
	}
	boot := make([]byte, 512)
	if _, err = a.ReadAt(boot, 2048*512); err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint16(boot[24:26]) != 63 ||
		binary.LittleEndian.Uint16(boot[26:28]) != 255 ||
		binary.LittleEndian.Uint32(boot[28:32]) != 2048 {
		t.Fatal("invalid FAT32 BPB geometry")
	}
	if binary.LittleEndian.Uint32(boot[67:71]) != diskSignature {
		t.Fatal("FAT32 volume ID does not match MBR disk signature")
	}
	fatSectors := int64(binary.LittleEndian.Uint32(boot[36:40]))
	firstDataSector := int64(2048) +
		int64(binary.LittleEndian.Uint16(boot[14:16])) +
		int64(boot[16])*fatSectors
	rootDir := make([]byte, 512)
	if _, err = a.ReadAt(rootDir, firstDataSector*512); err != nil {
		t.Fatal(err)
	}
	wbfsEntry := rootDir[32:64]
	wbfsCluster := uint32(binary.LittleEndian.Uint16(wbfsEntry[20:22]))<<16 |
		uint32(binary.LittleEndian.Uint16(wbfsEntry[26:28]))
	wbfsDirSector := firstDataSector + int64(wbfsCluster-2)*int64(boot[13])
	wbfsDir := make([]byte, 512)
	if _, err = a.ReadAt(wbfsDir, wbfsDirSector*512); err != nil {
		t.Fatal(err)
	}
	// Entries are ".", "..", one LFN, then the corresponding 8.3 file.
	fileAttr := wbfsDir[3*32+11]
	if fileAttr&0x01 != 0 {
		t.Fatalf("WBFS file has DOS read-only attribute %#x", fileAttr)
	}
	if fileAttr&0x20 == 0 {
		t.Fatalf("WBFS file is missing DOS archive attribute %#x", fileAttr)
	}
	otherCatalog, err := vdisk.Build("other-catalog", result.Games, "test")
	if err != nil {
		t.Fatal(err)
	}
	otherMBR := make([]byte, 512)
	if _, err = otherCatalog.ReadAt(otherMBR, 0); err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(otherMBR[440:444]) == diskSignature {
		t.Fatal("different catalogs reused an MBR disk signature")
	}
	if _, err = a.ReadAt(make([]byte, 513), a.Size()-512); err == nil {
		t.Fatal("out-of-range read accepted")
	}
}

func TestVirtualLargeFileSplitting(t *testing.T) {
	// Match USB Loader GX's one-32-KiB-cluster-below-4-GiB boundary so a
	// large-volume FAT chain never spans exactly 2^32 bytes.
	const firstSegment = int64(4<<30) - int64(32<<10)
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
