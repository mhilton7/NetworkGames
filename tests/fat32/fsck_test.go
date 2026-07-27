package fat32_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"wiibridge/server/host-daemon/scanner"
	"wiibridge/server/host-daemon/vdisk"
	"wiibridge/tests/testutil"
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
	// mtools exercises host-side FAT geometry parsing that fsck.vfat accepts
	// more leniently. Windows rejects a BPB with zero heads/sectors-per-track.
	if mdir := "/usr/bin/mdir"; fileExists(mdir) {
		command = exec.Command(mdir, "-i", imagePath+"@@1048576", "::")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("mtools directory read failed: %v\n%s", err, output)
		}
	}
	if mattrib := "/usr/bin/mattrib"; fileExists(mattrib) {
		command = exec.Command(
			mattrib, "-i", imagePath+"@@1048576", "::/wbfs/TEST02.wbfs",
		)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("mtools attribute read failed: %v\n%s", err, output)
		}
		attr, _, _ := strings.Cut(string(output), "::")
		if strings.Contains(attr, "R") {
			t.Fatalf("WBFS file is DOS read-only: %q", attr)
		}
		if !strings.Contains(attr, "A") {
			t.Fatalf("WBFS file is missing archive attribute: %q", attr)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
