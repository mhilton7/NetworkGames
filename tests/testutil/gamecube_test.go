package testutil

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestSyntheticGameCubeISOContainsOnlyMinimalFixtureMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.iso")
	if err := SyntheticGameCubeISO(path, "GTEST1", "Synthetic GameCube", 0, 2, 2<<20); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	header := make([]byte, 0x440)
	if _, err = f.ReadAt(header, 0); err != nil {
		t.Fatal(err)
	}
	if string(header[:6]) != "GTEST1" || header[6] != 0 || header[7] != 2 {
		t.Fatalf("unexpected synthetic identity: %q disc=%d revision=%d",
			header[:6], header[6], header[7])
	}
	if binary.BigEndian.Uint32(header[0x1c:0x20]) != GameCubeMagic {
		t.Fatal("synthetic fixture lacks GameCube magic")
	}
	if binary.BigEndian.Uint32(header[0x18:0x1c]) == GameCubeMagic {
		t.Fatal("synthetic fixture placed GameCube magic in the Wii magic field")
	}
	if got := binary.BigEndian.Uint32(header[0x424:0x428]); got != uint32(GameCubeFSTOffset) {
		t.Fatalf("synthetic fixture FST offset = %#x, want %#x", got, GameCubeFSTOffset)
	}
	if got := binary.BigEndian.Uint32(header[0x428:0x42c]); got != 12 {
		t.Fatalf("synthetic fixture FST size = %d, want 12", got)
	}
	if info, err := f.Stat(); err != nil || info.Size() != 2<<20 {
		t.Fatalf("unexpected synthetic fixture size: info=%v err=%v", info, err)
	}
}
