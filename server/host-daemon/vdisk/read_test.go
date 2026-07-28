package vdisk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"wiibridge/shared/model"
)

type limitedReaderAt struct {
	data  []byte
	limit int
	calls int
}

func (r *limitedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	r.calls++
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	if len(p) > r.limit {
		p = p[:r.limit]
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestReadAtFullRetriesShortReads(t *testing.T) {
	reader := &limitedReaderAt{data: []byte("0123456789abcdef"), limit: 3}
	got := make([]byte, 10)
	n, err := readAtFull(reader, got, 2)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(got) || string(got) != "23456789ab" {
		t.Fatalf("got n=%d data=%q", n, got)
	}
	if reader.calls < 2 {
		t.Fatalf("short reads were not retried: %d call(s)", reader.calls)
	}
}

func TestReadAtFullReportsEOFAndNoProgress(t *testing.T) {
	reader := &limitedReaderAt{data: []byte("short"), limit: 2}
	got := make([]byte, 8)
	n, err := readAtFull(reader, got, 0)
	if !errors.Is(err, io.EOF) || n != 5 {
		t.Fatalf("got n=%d err=%v, want 5, EOF", n, err)
	}
	n, err = readAtFull(readerAtFunc(func([]byte, int64) (int, error) {
		return 0, nil
	}), got, 0)
	if !errors.Is(err, io.ErrNoProgress) || n != 0 {
		t.Fatalf("got n=%d err=%v, want 0, ErrNoProgress", n, err)
	}
}

type readerAtFunc func([]byte, int64) (int, error)

func (f readerAtFunc) ReadAt(p []byte, off int64) (int, error) {
	return f(p, off)
}

func TestReadAtCrossesSourcePartAndSparseBoundaries(t *testing.T) {
	root := t.TempDir()
	first := writeSource(t, filepath.Join(root, "part.wbfs"), []byte("abcdefgh"))
	second := writeSource(t, filepath.Join(root, "part.wbf1"), []byte("IJKLMNOP"))
	disk := &Disk{
		size:     24,
		metadata: map[int64][]byte{},
		extents: []extent{
			{start: 0, length: 8, source: &first},
			{start: 8, length: 4, zero: true},
			{start: 12, length: 8, source: &second},
			{start: 20, length: 4, zero: true},
		},
	}
	got := make([]byte, 20)
	n, err := disk.ReadAt(got, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte("efgh"), make([]byte, 4)...)
	want = append(want, []byte("IJKLMNOP")...)
	want = append(want, make([]byte, 4)...)
	if n != len(got) || !bytes.Equal(got, want) {
		t.Fatalf("cross-boundary read got n=%d %q, want %q", n, got, want)
	}
	if _, err = disk.ReadAt(make([]byte, 1), disk.Size()); !errors.Is(err, io.EOF) {
		t.Fatalf("final-sector bound accepted: %v", err)
	}
}

func TestIndependentDisksAllowConcurrentSelectionAndReads(t *testing.T) {
	root := t.TempDir()
	a := writeSource(t, filepath.Join(root, "a.wbfs"), bytes.Repeat([]byte{0xa5}, 4096))
	b := writeSource(t, filepath.Join(root, "b.wbfs"), bytes.Repeat([]byte{0x5a}, 4096))
	disks := []*Disk{
		{size: 4096, metadata: map[int64][]byte{}, extents: []extent{{start: 0, length: 4096, source: &a}}},
		{size: 4096, metadata: map[int64][]byte{}, extents: []extent{{start: 0, length: 4096, source: &b}}},
	}
	var wg sync.WaitGroup
	for iteration := 0; iteration < 20; iteration++ {
		for diskIndex, disk := range disks {
			wg.Add(1)
			go func(index int, selected *Disk) {
				defer wg.Done()
				buf := make([]byte, 4096)
				if _, err := selected.ReadAt(buf, 0); err != nil {
					t.Errorf("disk %d: %v", index, err)
					return
				}
				want := byte(0xa5)
				if index == 1 {
					want = 0x5a
				}
				if !bytes.Equal(buf, bytes.Repeat([]byte{want}, len(buf))) {
					t.Errorf("disk %d returned mixed selection data", index)
				}
			}(diskIndex, disk)
		}
	}
	wg.Wait()
}

func TestLargeVirtualFATIsSynthesizedInsteadOfResident(t *testing.T) {
	const payloadSize = int64(8 << 30)
	game := model.Game{
		ID: "MEM001", Size: payloadSize,
		Sources: []model.Source{{
			Path: "/synthetic/not-opened", Length: payloadSize, Size: payloadSize,
		}},
	}
	disk, err := Build("memory-bound", []model.Game{game}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if disk.fatSectors < 16_000 {
		t.Fatalf("fixture FAT is too small to exercise memory behavior: %d sectors",
			disk.fatSectors)
	}
	if len(disk.metadata) > 32 {
		t.Fatalf("FAT sectors were materialized in metadata: %d resident sectors",
			len(disk.metadata))
	}
	if len(disk.fatChains) > 16 {
		t.Fatalf("FAT chain representation scales with clusters: %d chains",
			len(disk.fatChains))
	}
	t.Logf("8 GiB fixture: virtual FAT sectors per copy=%d compact chains=%d resident metadata sectors=%d",
		disk.fatSectors, len(disk.fatChains), len(disk.metadata))
	first := make([]byte, sectorSize)
	second := make([]byte, sectorSize)
	if _, err = disk.ReadAt(first, disk.fatStart*sectorSize); err != nil {
		t.Fatal(err)
	}
	if _, err = disk.ReadAt(second,
		(disk.fatStart+disk.fatSectors)*sectorSize); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) ||
		binary.LittleEndian.Uint32(first[:4]) != 0x0ffffff8 ||
		binary.LittleEndian.Uint32(first[4:8]) != 0xffffffff {
		t.Fatal("synthetic FAT copies differ or have invalid reserved entries")
	}
}

func TestLargeVirtualFATBuildDoesNotWalkApparentFATSectors(t *testing.T) {
	const payloadSize = int64(512 << 30)
	game := model.Game{
		ID: "FAST01", Size: payloadSize,
		Sources: []model.Source{{
			Path: "/synthetic/not-opened", Length: payloadSize, Size: payloadSize,
		}},
	}
	started := time.Now()
	disk, err := Build("bounded-startup", []model.Game{game}, "test")
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	if disk.fatSectors < 1_000_000 {
		t.Fatalf("fixture FAT is too small to exercise bounded startup: %d sectors",
			disk.fatSectors)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("compact metadata identity walked the apparent FAT: build took %s",
			elapsed)
	}
	if disk.Snapshot().MetadataHash != disk.metadataIdentity() {
		t.Fatal("snapshot does not use the compact deterministic metadata identity")
	}
	t.Logf("512 GiB fixture: FAT sectors per copy=%d chains=%d build=%s",
		disk.fatSectors, len(disk.fatChains), elapsed)
}

func TestCompactMetadataIdentityChangesWithFATLayout(t *testing.T) {
	base := &Disk{
		size: 4096, fatStart: 32, fatSectors: 64,
		metadata:  map[int64][]byte{0: bytes.Repeat([]byte{0xa5}, 512)},
		fatChains: []clusterChain{{first: 2, count: 3}},
	}
	same := &Disk{
		size: base.size, fatStart: base.fatStart, fatSectors: base.fatSectors,
		metadata:  map[int64][]byte{0: append([]byte(nil), base.metadata[0]...)},
		fatChains: append([]clusterChain(nil), base.fatChains...),
	}
	if base.metadataIdentity() != same.metadataIdentity() {
		t.Fatal("equivalent compact layouts produced different identities")
	}
	same.fatChains[0].count++
	if base.metadataIdentity() == same.metadataIdentity() {
		t.Fatal("different FAT chains produced the same compact identity")
	}
}

func writeSource(t *testing.T, path string, data []byte) model.Source {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	return model.Source{
		Path: path, Length: info.Size(), Size: info.Size(),
		ModUnix: info.ModTime().UnixNano(), Device: uint64(stat.Dev), Inode: stat.Ino,
	}
}
