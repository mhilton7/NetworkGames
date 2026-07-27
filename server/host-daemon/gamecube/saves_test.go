package gamecube

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"

	"wiibridge/tests/testutil"
)

func saveTestVolume(t *testing.T) VolumeManifest {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "library", "game.iso")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := testutil.SyntheticGameCubeISO(source, "GSAV01", "Save Test", 0, 0, 2<<20); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(filepath.Dir(source))
	if err != nil || len(result.Games) != 1 {
		t.Fatalf("scan: games=%d err=%v", len(result.Games), err)
	}
	manifest, err := BuildVolume(context.Background(), filepath.Join(root, "cache"),
		result.Games[0], MemoryCardEmulated)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func writeCard(t *testing.T, manifest VolumeManifest, name string, data []byte) {
	t.Helper()
	disk, err := diskfs.Open(manifest.ImagePath, diskfs.WithOpenMode(diskfs.ReadWrite))
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	fat, err := disk.GetFilesystem(1)
	if err != nil {
		t.Fatal(err)
	}
	defer fat.Close()
	file, err := fat.OpenFile("saves/"+name, os.O_CREATE|os.O_RDWR|os.O_TRUNC)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}

func readCard(t *testing.T, manifest VolumeManifest, name string) []byte {
	t.Helper()
	disk, err := diskfs.Open(manifest.ImagePath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	fat, err := disk.GetFilesystem(1)
	if err != nil {
		t.Fatal(err)
	}
	defer fat.Close()
	data, err := readSave(fat, name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMemoryCardBackupRotationAndRestore(t *testing.T) {
	manifest := saveTestVolume(t)
	backupRoot := filepath.Join(t.TempDir(), "backups")
	original := bytes.Repeat([]byte{0x41}, 512<<10)
	writeCard(t, manifest, "GSAV.raw", original)

	var restorePath string
	for index := 0; index < 7; index++ {
		backups, err := BackupMemoryCards(manifest, backupRoot, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(backups) != 1 {
			t.Fatalf("backup count = %d, want 1", len(backups))
		}
		restorePath = backups[0].Path
	}
	directory := filepath.Dir(restorePath)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("retained backups = %d, want 5", len(entries))
	}

	writeCard(t, manifest, "GSAV.raw", bytes.Repeat([]byte{0x42}, 512<<10))
	if err = RestoreMemoryCard(manifest, restorePath, "GSAV.raw"); err != nil {
		t.Fatal(err)
	}
	if got := readCard(t, manifest, "GSAV.raw"); !bytes.Equal(got, original) {
		t.Fatal("restored memory card differs from selected backup")
	}
}

func TestMemoryCardBackupRejectsZeroByteAndUnsafeRestore(t *testing.T) {
	manifest := saveTestVolume(t)
	writeCard(t, manifest, "GSAV.raw", nil)
	if _, err := BackupMemoryCards(manifest, t.TempDir(), 5); err == nil {
		t.Fatal("zero-byte memory card was backed up")
	}
	if err := RestoreMemoryCard(manifest, filepath.Join(t.TempDir(), "missing"), "../escape.raw"); err == nil {
		t.Fatal("unsafe save name was accepted")
	}
}

func TestPhysicalCardModeDoesNotTouchUSBSaves(t *testing.T) {
	manifest := saveTestVolume(t)
	manifest.Mode = MemoryCardPhysical
	backups, err := BackupMemoryCards(manifest, t.TempDir(), 5)
	if err != nil || len(backups) != 0 {
		t.Fatalf("physical card backup = %#v, %v", backups, err)
	}
}

func TestInterruptedBackupIsNeverOfferedForRestore(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "GSAV01", "r0", "GSAV.raw")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".save-backup-interrupted"),
		bytes.Repeat([]byte{0x43}, 512<<10), 0o600); err != nil {
		t.Fatal(err)
	}
	backups, err := ListSaveBackups(root, "GSAV01", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("partial backup was offered for restore: %#v", backups)
	}
}
