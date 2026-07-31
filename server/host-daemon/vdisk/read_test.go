package vdisk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	if disk.sectorsPerCluster != 8 {
		t.Fatalf("small-library geometry changed to %d sectors per cluster",
			disk.sectorsPerCluster)
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
	if disk.fatSectors < 100_000 {
		t.Fatalf("fixture FAT is too small to exercise bounded startup: %d sectors",
			disk.fatSectors)
	}
	if disk.sectorsPerCluster != 64 {
		t.Fatalf("large Wii volume selected %d sectors per cluster, want 64",
			disk.sectorsPerCluster)
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

func TestLiveScaleSelectsValidFAT32Geometry(t *testing.T) {
	// This synthetic payload reproduces the scale of the 1,816,313,603,072-byte
	// export that USB Loader GX could not initialize. A fixed 4 KiB cluster
	// layout needs more FAT32 data clusters than the format can represent.
	const payloadSize = int64(1_812_700_000_000)
	game := model.Game{
		ID: "BIG001", Size: payloadSize,
		Sources: []model.Source{{
			Path: "/synthetic/not-opened", Length: payloadSize, Size: payloadSize,
		}},
	}
	disk, err := Build("live-scale-regression", []model.Game{game}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if disk.sectorsPerCluster != 64 {
		t.Fatalf("selected %d sectors per cluster, want 64", disk.sectorsPerCluster)
	}

	mbrSector := make([]byte, sectorSize)
	if _, err = disk.ReadAt(mbrSector, 0); err != nil {
		t.Fatal(err)
	}
	partitionSectors := int64(binary.LittleEndian.Uint32(mbrSector[458:462]))
	boot := make([]byte, sectorSize)
	if _, err = disk.ReadAt(boot, partitionStart*sectorSize); err != nil {
		t.Fatal(err)
	}
	sectorsPerCluster := int64(boot[13])
	fatSectors := int64(binary.LittleEndian.Uint32(boot[36:40]))
	firstData := int64(binary.LittleEndian.Uint16(boot[14:16])) +
		int64(boot[16])*fatSectors
	dataClusters := (partitionSectors - firstData) / sectorsPerCluster
	if dataClusters > maxFAT32DataClusters {
		t.Fatalf("FAT32 geometry exposes %d data clusters, maximum is %d",
			dataClusters, maxFAT32DataClusters)
	}
	info := make([]byte, sectorSize)
	if _, err = disk.ReadAt(info, (partitionStart+1)*sectorSize); err != nil {
		t.Fatal(err)
	}
	freeClusters := binary.LittleEndian.Uint32(info[488:492])
	nextFreeCluster := binary.LittleEndian.Uint32(info[492:496])
	var allocatedClusters uint32
	for _, chain := range disk.fatChains {
		allocatedClusters += chain.count
	}
	if freeClusters != uint32(dataClusters)-allocatedClusters {
		t.Fatalf("FSInfo reports %d free clusters, want %d",
			freeClusters, uint32(dataClusters)-allocatedClusters)
	}
	if freeClusters == 0 || freeClusters == 0xffffffff {
		t.Fatalf("FSInfo uses libfat full-scan sentinel %#x", freeClusters)
	}
	if nextFreeCluster != 2+allocatedClusters {
		t.Fatalf("FSInfo next-free cluster is %d, want %d",
			nextFreeCluster, 2+allocatedClusters)
	}
	if disk.fatValue(nextFreeCluster) != 0 {
		t.Fatalf("FSInfo next-free cluster %d is allocated", nextFreeCluster)
	}
	backupInfo := make([]byte, sectorSize)
	if _, err = disk.ReadAt(backupInfo, (partitionStart+7)*sectorSize); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(info, backupInfo) {
		t.Fatal("primary and backup FSInfo sectors differ")
	}
	if fatSectors > 500_000 {
		t.Fatalf("large-volume FAT is too large for the Wii mount path: %d sectors",
			fatSectors)
	}
	if disk.Size()/sectorSize > maxLBA32DiskSectors {
		t.Fatalf("disk uses %d sectors, 32-bit LBA maximum is %d",
			disk.Size()/sectorSize, maxLBA32DiskSectors)
	}
	t.Logf("live-scale fixture: size=%d sectors/cluster=%d data clusters=%d FAT sectors/copy=%d",
		disk.Size(), sectorsPerCluster, dataClusters, fatSectors)
}

func TestLargeWBFSVirtualSegmentsUseUSBLoaderGXBoundary(t *testing.T) {
	// USB Loader GX r1283 deliberately keeps each FAT32 split one 32 KiB
	// cluster below 4 GiB. A 4 GiB-minus-4 KiB file rounds up to exactly
	// 2^32 bytes on WiiBridge's large-volume 32 KiB geometry, which overflows
	// 32-bit FAT chain-length accounting in compatibility tools and the Wii
	// storage stack.
	const loaderSplitSize = int64(4<<30) - 32<<10
	const payloadSize = int64(32<<30) + 512
	game := model.Game{
		ID: "SPL001", Size: payloadSize,
		Sources: []model.Source{{
			Path: "/synthetic/not-opened", Length: payloadSize, Size: payloadSize,
		}},
	}
	disk, err := Build("usbloadergx-split-boundary", []model.Game{game}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if disk.sectorsPerCluster != 64 {
		t.Fatalf("selected %d sectors per cluster, want 64", disk.sectorsPerCluster)
	}
	files := disk.Snapshot().VirtualFiles
	if len(files) != 9 {
		t.Fatalf("created %d split segments, want 9", len(files))
	}
	firstDataSector := partitionStart + reservedSectors + numFATs*disk.fatSectors
	wbfsDirectoryOffset := (firstDataSector +
		int64(disk.fatChains[1].first-2)*disk.sectorsPerCluster) * sectorSize
	directory := make([]byte, (2+2*len(files))*32)
	if _, err = disk.ReadAt(directory, wbfsDirectoryOffset); err != nil {
		t.Fatal(err)
	}
	shortAliases := make(map[string]struct{}, len(files))
	for index, file := range files {
		wantSuffix := ".wbfs"
		if index > 0 {
			wantSuffix = fmt.Sprintf(".wbf%d", index)
		}
		if !strings.HasSuffix(file.Path, wantSuffix) {
			t.Fatalf("segment %d path %q does not end in %q", index, file.Path, wantSuffix)
		}
		if index < len(files)-1 && file.Length != loaderSplitSize {
			t.Fatalf("segment %d length=%d, want USB Loader GX boundary %d",
				index, file.Length, loaderSplitSize)
		}
		if index > 0 && file.LogicalStart != files[index-1].LogicalStart+files[index-1].Length {
			t.Fatalf("segment %d does not immediately follow segment %d", index, index-1)
		}
		lfn := directory[(2+2*index)*32 : (3+2*index)*32]
		short := directory[(3+2*index)*32 : (4+2*index)*32]
		if short[11] != 0x20 {
			t.Fatalf("segment %d attributes=%#x, want archive-only 0x20", index, short[11])
		}
		if lfn[13] != lfnChecksum(short[:11]) {
			t.Fatalf("segment %d has invalid LFN checksum", index)
		}
		alias := string(short[:11])
		if _, exists := shortAliases[alias]; exists {
			t.Fatalf("segment %d reuses short alias %q", index, alias)
		}
		shortAliases[alias] = struct{}{}
		if binary.LittleEndian.Uint32(short[28:32]) != uint32(file.Length) {
			t.Fatalf("segment %d directory size does not match snapshot", index)
		}
	}
	clusterSize := disk.sectorsPerCluster * sectorSize
	// FAT chains are root, /wbfs, then one chain per virtual segment.
	firstSegmentCapacity := int64(disk.fatChains[2].count) * clusterSize
	if firstSegmentCapacity >= int64(1<<32) {
		t.Fatalf("first segment FAT chain spans %d bytes; must remain below 2^32",
			firstSegmentCapacity)
	}
}

func TestLargeWBFSBannerAndSplitBoundaryReadsMatchSource(t *testing.T) {
	const payloadSize = int64(32<<30) + 512
	const markerSize = 4096
	root := t.TempDir()
	path := filepath.Join(root, "SPL003.wbfs")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(payloadSize); err != nil {
		file.Close()
		t.Fatal(err)
	}
	bannerOffset := int64(0x10000)
	markers := []struct {
		offset int64
		value  byte
	}{
		{offset: bannerOffset, value: 0xb3},
		{offset: maxSegment - markerSize/2, value: 0xa1},
		{offset: maxSegment, value: 0xc2},
	}
	for _, marker := range markers {
		if _, err = file.WriteAt(bytes.Repeat([]byte{marker.value}, markerSize), marker.offset); err != nil {
			file.Close()
			t.Fatal(err)
		}
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	source := model.Source{
		Path: path, Length: payloadSize, Size: payloadSize,
		ModUnix: info.ModTime().UnixNano(), Device: uint64(stat.Dev), Inode: stat.Ino,
	}
	disk, err := Build("banner-and-split-boundary", []model.Game{{
		ID: "SPL003", Size: payloadSize, Sources: []model.Source{source},
	}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	var payloadExtents []extent
	for _, item := range disk.extents {
		if !item.zero {
			payloadExtents = append(payloadExtents, item)
		}
	}
	if len(payloadExtents) < 2 {
		t.Fatalf("created %d payload extents, want at least two", len(payloadExtents))
	}
	banner := make([]byte, markerSize)
	if _, err = disk.ReadAt(banner, payloadExtents[0].start+bannerOffset); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(banner, bytes.Repeat([]byte{0xb3}, len(banner))) {
		t.Fatal("banner-region virtual bytes differ from source")
	}
	boundary := make([]byte, markerSize)
	if _, err = disk.ReadAt(boundary, payloadExtents[0].start+maxSegment-markerSize/2); err != nil {
		t.Fatal(err)
	}
	want := append(bytes.Repeat([]byte{0xa1}, markerSize/2),
		bytes.Repeat([]byte{0xc2}, markerSize/2)...)
	if !bytes.Equal(boundary, want) {
		t.Fatal("read crossing the virtual WBFS split boundary differs from source")
	}
}

func TestLargeWBFSVirtualSegmentsPassIndependentFsck(t *testing.T) {
	const fsck = "/usr/sbin/fsck.vfat"
	if _, err := os.Stat(fsck); err != nil {
		t.Skip("fsck.vfat unavailable")
	}
	const payloadSize = int64(32<<30) + 512
	game := model.Game{
		ID: "SPL002", Size: payloadSize,
		Sources: []model.Source{{
			Path: "/synthetic/not-opened", Length: payloadSize, Size: payloadSize,
		}},
	}
	disk, err := Build("usbloadergx-split-fsck", []model.Game{game}, "test")
	if err != nil {
		t.Fatal(err)
	}
	partitionPath := filepath.Join(t.TempDir(), "large-sparse-partition.img")
	partition, err := os.Create(partitionPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = partition.Truncate(disk.Size() - partitionStart*sectorSize); err != nil {
		partition.Close()
		t.Fatal(err)
	}
	err = disk.forEachMetadataSector(func(sector int64, data []byte) error {
		if sector < partitionStart {
			return nil
		}
		_, writeErr := partition.WriteAt(data, (sector-partitionStart)*sectorSize)
		return writeErr
	})
	if closeErr := partition.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(fsck, "-n", partitionPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("independent fsck rejected sparse large-volume splits: %v\n%s", err, output)
	}
}

func TestCompactMetadataIdentityChangesWithFATLayout(t *testing.T) {
	base := &Disk{
		size: 4096, sectorsPerCluster: 8, fatStart: 32, fatSectors: 64,
		metadata:  map[int64][]byte{0: bytes.Repeat([]byte{0xa5}, 512)},
		fatChains: []clusterChain{{first: 2, count: 3}},
	}
	same := &Disk{
		size: base.size, sectorsPerCluster: base.sectorsPerCluster,
		fatStart: base.fatStart, fatSectors: base.fatSectors,
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
	same.fatChains[0].count = base.fatChains[0].count
	same.sectorsPerCluster *= 2
	if base.metadataIdentity() == same.metadataIdentity() {
		t.Fatal("different sectors-per-cluster values produced the same compact identity")
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
