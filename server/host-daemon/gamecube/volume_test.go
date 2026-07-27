package gamecube

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"

	"wiibridge/tests/testutil"
)

func TestBuildSingleDiscVolumeAndReuseValidatedCache(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "library", "game.iso")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.SyntheticGameCubeISO(source, "GVOL01", "Volume Test", 0, 0, 2<<20); err != nil {
		t.Fatal(err)
	}
	scan, err := Scan(filepath.Dir(source))
	if err != nil || len(scan.Games) != 1 {
		t.Fatalf("scan: games=%d err=%v", len(scan.Games), err)
	}
	cache := filepath.Join(root, "cache")
	manifest, err := BuildVolume(context.Background(), cache, scan.Games[0], MemoryCardPhysical)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Game.Discs[0].SHA256) != 64 {
		t.Fatal("Prepare export did not hash the source image")
	}
	validation, err := ValidateVolume(manifest.ImagePath, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if validation.ClusterSize != 32<<10 || !validation.BackupBoot ||
		!validation.RequiredPathsOK || validation.Capacity != VolumeSize {
		t.Fatalf("invalid volume validation: %#v", validation)
	}
	info, err := os.Stat(manifest.ImagePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != VolumeSize {
		t.Fatalf("unexpected virtual capacity: %d", info.Size())
	}
	disk, err := diskfs.Open(manifest.ImagePath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	fat, err := disk.GetFilesystem(1)
	if err != nil {
		disk.Close()
		t.Fatal(err)
	}
	gamePath := strings.TrimPrefix(manifest.RuntimePaths[0], "/")
	file, err := fat.Open(gamePath)
	if err != nil {
		fat.Close()
		disk.Close()
		t.Fatal(err)
	}
	header := make([]byte, 6)
	if _, err = file.Read(header); err != nil {
		t.Fatal(err)
	}
	file.Close()
	fat.Close()
	disk.Close()
	if string(header) != "GVOL01" {
		t.Fatalf("staged source mismatch: %q", header)
	}
	before := manifest.Created
	reused, err := BuildVolume(context.Background(), cache, scan.Games[0], MemoryCardPhysical)
	if err != nil {
		t.Fatal(err)
	}
	if reused.CacheKey != manifest.CacheKey || !reused.Created.Equal(before) {
		t.Fatalf("validated cache was rebuilt: before=%v after=%v", manifest, reused)
	}
}

func TestBuildTwoDiscLayout(t *testing.T) {
	root := t.TempDir()
	for disc := byte(0); disc < 2; disc++ {
		path := filepath.Join(root, "library", "disc"+string(rune('1'+disc))+".iso")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := testutil.SyntheticGameCubeISO(path, "G2DS01", "Two Disc", disc, 1, 2<<20); err != nil {
			t.Fatal(err)
		}
	}
	scan, err := Scan(filepath.Join(root, "library"))
	if err != nil || len(scan.Games) != 1 || scan.Games[0].DiscCount != 2 {
		t.Fatalf("scan: %#v err=%v", scan, err)
	}
	manifest, err := BuildVolume(context.Background(), filepath.Join(root, "cache"),
		scan.Games[0], MemoryCardEmulated)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.RuntimePaths) != 2 ||
		filepath.Base(manifest.RuntimePaths[0]) != "game.iso" ||
		filepath.Base(manifest.RuntimePaths[1]) != "disc2.iso" {
		t.Fatalf("unexpected two-disc paths: %#v", manifest.RuntimePaths)
	}
	if _, err = ValidateVolume(manifest.ImagePath, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRejectsIncompleteGameAndInvalidMode(t *testing.T) {
	game := Game{ID: "GFAIL1", Validation: "invalid"}
	if _, err := BuildVolume(context.Background(), t.TempDir(), game, MemoryCardPhysical); err == nil {
		t.Fatal("invalid game was exported")
	}
	game.Validation = "valid"
	game.Discs = []Disc{{ID: game.ID, Validation: "valid"}}
	if _, err := BuildVolume(context.Background(), t.TempDir(), game, "unknown"); err == nil {
		t.Fatal("invalid memory-card mode was accepted")
	}
}

func TestFileBackendEnforcesSelectedWriteMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "volume.img")
	if err := os.WriteFile(path, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	readOnly, err := OpenFileBackend(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if !readOnly.ReadOnly() {
		t.Fatal("physical-card backend is not read-only")
	}
	if _, err = readOnly.WriteAt([]byte("x"), 0); err == nil {
		t.Fatal("read-only backend accepted a write")
	}
	readOnly.Close()
	writable, err := OpenFileBackend(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writable.Close()
	if writable.ReadOnly() {
		t.Fatal("emulated-card backend is read-only")
	}
	if _, err = writable.WriteAt([]byte("X"), 0); err != nil {
		t.Fatal(err)
	}
	if err = writable.Sync(); err != nil {
		t.Fatal(err)
	}
}
