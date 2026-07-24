package fat32_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"networkgames/server/host-daemon/scanner"
	"networkgames/server/host-daemon/vdisk"
	"networkgames/tests/testutil"
)

func TestSynthesizedFAT32WithIndependentFsck(t *testing.T) {
	const fsck = "/usr/sbin/fsck.vfat"
	if _, err := os.Stat(fsck); err != nil {
		t.Skip("fsck.vfat unavailable")
	}
	root := t.TempDir()
	if err := testutil.SyntheticWBFS(filepath.Join(root, "TEST02.wbfs"), "TEST02", "FAT32 synthetic", 2<<20); err != nil {
		t.Fatal(err)
	}
	result, err := scanner.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	disk, err := vdisk.Build("fsck", result.Games, "test")
	if err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(root, "reference.img")
	image, err := os.Create(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := image.Truncate(disk.Size()); err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 4096)
	zero := make([]byte, len(block))
	for off := int64(0); off < disk.Size(); off += int64(len(block)) {
		n := int64(len(block))
		if n > disk.Size()-off {
			n = disk.Size() - off
		}
		if _, err := disk.ReadAt(block[:n], off); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(block[:n], zero[:n]) {
			if _, err := image.WriteAt(block[:n], off); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := image.Close(); err != nil {
		t.Fatal(err)
	}
	partitionPath := filepath.Join(root, "reference-partition.img")
	partition, err := os.Create(partitionPath)
	if err != nil {
		t.Fatal(err)
	}
	const partitionOffset = int64(2048 * 512)
	if err := partition.Truncate(disk.Size() - partitionOffset); err != nil {
		t.Fatal(err)
	}
	for off := partitionOffset; off < disk.Size(); off += int64(len(block)) {
		n := int64(len(block))
		if n > disk.Size()-off {
			n = disk.Size() - off
		}
		if _, err := disk.ReadAt(block[:n], off); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(block[:n], zero[:n]) {
			if _, err := partition.WriteAt(block[:n], off-partitionOffset); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := partition.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(fsck, "-n", partitionPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fsck failed: %v\n%s", err, output)
	}
}
