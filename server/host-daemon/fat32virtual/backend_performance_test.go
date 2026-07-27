package fat32virtual

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
)

func performanceSource(t testing.TB, root string, index int, size int64) File {
	t.Helper()
	source := filepath.Join(root, fmt.Sprintf("source-%05d.iso", index))
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(size); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	zeroHash := sha256.New()
	buffer := make([]byte, 64<<10)
	for remaining := size; remaining > 0; {
		count := int64(len(buffer))
		if remaining < count {
			count = remaining
		}
		_, _ = zeroHash.Write(buffer[:count])
		remaining -= count
	}
	identity := Identity{
		Size: info.Size(), ModTimeUnixNano: info.ModTime().UnixNano(),
		SHA256: hex.EncodeToString(zeroHash.Sum(nil)),
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		identity.Device, identity.Inode = uint64(stat.Dev), stat.Ino
	}
	return File{
		SourcePath: source, LogicalSize: size, SourceSize: size, Identity: identity,
	}
}

func performanceLayout(sources []File, count int, extentSize int64) Layout {
	extents := make([]Extent, count)
	const start = int64(1 << 20)
	for index := range extents {
		source := sources[index%len(sources)]
		extents[index] = Extent{
			VirtualOffset: start + int64(index)*extentSize,
			Length:        extentSize, SourcePath: source.SourcePath,
			SourceSize: source.SourceSize, Identity: source.Identity, ReadOnly: true,
		}
	}
	empty := sha256.Sum256(nil)
	return Layout{
		Schema: 2, VirtualSize: start + int64(count)*extentSize + extentSize,
		SourceExtents: extents, MetadataHash: hex.EncodeToString(empty[:]),
		ExtentMapHash: hashExtents(extents),
	}
}

func TestSourceHandleCacheIsBoundedAndReused(t *testing.T) {
	root := t.TempDir()
	var sources []File
	for index := 0; index < 40; index++ {
		sources = append(sources, performanceSource(t, root, index, 4096))
	}
	layout := performanceLayout(sources, len(sources), 4096)
	backend, err := Open(layout, nil, 32)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4096)
	for index := 0; index < 100; index++ {
		if _, err = backend.ReadAt(buffer, layout.SourceExtents[0].VirtualOffset); err != nil {
			t.Fatal(err)
		}
	}
	stats := backend.Stats()
	if stats.SourceOpens != 1 || stats.SourceReads != 100 || stats.CacheHits != 99 {
		t.Fatalf("single-source cache stats=%+v", stats)
	}
	for _, extent := range layout.SourceExtents[1:] {
		if _, err = backend.ReadAt(buffer, extent.VirtualOffset); err != nil {
			t.Fatal(err)
		}
	}
	stats = backend.Stats()
	if stats.CachedFiles != 32 || stats.PeakOpenFiles != 32 || stats.SourceOpens != 40 {
		t.Fatalf("bounded cache stats=%+v", stats)
	}
	if err = backend.Close(); err != nil {
		t.Fatal(err)
	}
	if stats = backend.Stats(); stats.CachedFiles != 0 {
		t.Fatalf("backend close retained handles: %+v", stats)
	}
}

func TestLargeReadInsideExtentIsCoalesced(t *testing.T) {
	source := performanceSource(t, t.TempDir(), 0, 2<<20)
	layout := performanceLayout([]File{source}, 1, source.LogicalSize)
	backend, err := Open(layout, nil, 32)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	buffer := make([]byte, 1<<20)
	if _, err = backend.ReadAt(buffer, layout.SourceExtents[0].VirtualOffset+512); err != nil {
		t.Fatal(err)
	}
	stats := backend.Stats()
	if stats.SourceReads != 1 || stats.SourceOpens != 1 {
		t.Fatalf("1 MiB request was split or reopened: %+v", stats)
	}
}

func TestTenThousandExtentReadsStayMemoryBounded(t *testing.T) {
	root := t.TempDir()
	var sources []File
	for index := 0; index < 32; index++ {
		sources = append(sources, performanceSource(t, root, index, 4096))
	}
	layout := performanceLayout(sources, 10_000, 4096)
	backend, err := Open(layout, nil, 32)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	runtime.GC()
	var baseline, sample runtime.MemStats
	runtime.ReadMemStats(&baseline)
	peak := baseline.HeapAlloc
	buffer := make([]byte, 4096)
	for index := 0; index < 10_000; index++ {
		extent := layout.SourceExtents[(index*7919)%len(layout.SourceExtents)]
		if _, err = backend.ReadAt(buffer, extent.VirtualOffset); err != nil {
			t.Fatal(err)
		}
		if index%250 == 0 {
			runtime.ReadMemStats(&sample)
			if sample.HeapAlloc > peak {
				peak = sample.HeapAlloc
			}
		}
	}
	stats := backend.Stats()
	growth := peak - baseline.HeapAlloc
	if growth > 32<<20 || stats.PeakOpenFiles > 32 {
		t.Fatalf("unbounded hot-path resources: heap_growth=%d stats=%+v", growth, stats)
	}
	t.Logf("10,000 extents: peak_heap_growth=%d source_opens=%d source_reads=%d cache_hits=%d peak_open_files=%d",
		growth, stats.SourceOpens, stats.SourceReads, stats.CacheHits, stats.PeakOpenFiles)
}

func reportBackendMetrics(b *testing.B, backend *Backend) {
	stats := backend.Stats()
	b.ReportMetric(float64(stats.SourceOpens), "source-opens")
	b.ReportMetric(float64(stats.SourceReads)/float64(b.N), "source-reads/op")
	hitRate := float64(0)
	if stats.SourceReads > 0 {
		hitRate = 100 * float64(stats.CacheHits) / float64(stats.SourceReads)
	}
	b.ReportMetric(hitRate, "cache-hit-%")
	b.ReportMetric(float64(stats.PeakOpenFiles), "peak-open-files")
}

func BenchmarkSequentialReadsOneISO(b *testing.B) {
	source := performanceSource(b, b.TempDir(), 0, 2<<20)
	layout := performanceLayout([]File{source}, 1, source.LogicalSize)
	backend, err := Open(layout, nil, 32)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	buffer := make([]byte, 128<<10)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		offset := int64(index*(128<<10)) % (source.LogicalSize - int64(len(buffer)))
		if _, err = backend.ReadAt(buffer, layout.SourceExtents[0].VirtualOffset+offset); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	reportBackendMetrics(b, backend)
}

func BenchmarkRandomReadsOneISO(b *testing.B) {
	source := performanceSource(b, b.TempDir(), 0, 2<<20)
	layout := performanceLayout([]File{source}, 1, source.LogicalSize)
	backend, err := Open(layout, nil, 32)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	buffer := make([]byte, 4096)
	state := uint64(1)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		state = state*6364136223846793005 + 1
		offset := int64(state % uint64(source.LogicalSize-int64(len(buffer))))
		if _, err = backend.ReadAt(buffer, layout.SourceExtents[0].VirtualOffset+offset); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	reportBackendMetrics(b, backend)
}

func BenchmarkSwitchingThirtyTwoSources(b *testing.B) {
	root := b.TempDir()
	var sources []File
	for index := 0; index < 32; index++ {
		sources = append(sources, performanceSource(b, root, index, 4096))
	}
	layout := performanceLayout(sources, len(sources), 4096)
	backend, err := Open(layout, nil, 32)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	buffer := make([]byte, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		extent := layout.SourceExtents[index%len(layout.SourceExtents)]
		if _, err = backend.ReadAt(buffer, extent.VirtualOffset); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	reportBackendMetrics(b, backend)
}

func BenchmarkExtentLookup10000(b *testing.B) {
	source := File{SourcePath: "/not-opened", SourceSize: 4096}
	layout := performanceLayout([]File{source}, 10_000, 4096)
	position := layout.SourceExtents[len(layout.SourceExtents)-1].VirtualOffset
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, ok := findSource(layout.SourceExtents, position); !ok {
			b.Fatal("extent not found")
		}
	}
}

func BenchmarkExtractedFSTFiveThousandFiles(b *testing.B) {
	source := performanceSource(b, b.TempDir(), 0, 4096)
	layout := performanceLayout([]File{source}, 5_000, 4096)
	backend, err := Open(layout, nil, 32)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	buffer := make([]byte, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		extent := layout.SourceExtents[(index*3571)%len(layout.SourceExtents)]
		if _, err = backend.ReadAt(buffer, extent.VirtualOffset); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	reportBackendMetrics(b, backend)
}

func BenchmarkReadCrossingSourceExtents(b *testing.B) {
	root := b.TempDir()
	sources := []File{
		performanceSource(b, root, 0, 64<<10),
		performanceSource(b, root, 1, 64<<10),
	}
	layout := performanceLayout(sources, 2, 64<<10)
	backend, err := Open(layout, nil, 32)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	buffer := make([]byte, 128<<10)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err = backend.ReadAt(buffer, layout.SourceExtents[0].VirtualOffset); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	reportBackendMetrics(b, backend)
}

func BenchmarkConcurrentReads(b *testing.B) {
	source := performanceSource(b, b.TempDir(), 0, 2<<20)
	layout := performanceLayout([]File{source}, 1, source.LogicalSize)
	backend, err := Open(layout, nil, 32)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	var sequence atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		buffer := make([]byte, 4096)
		for pb.Next() {
			index := sequence.Add(1)
			offset := int64(index*4096) % (source.LogicalSize - int64(len(buffer)))
			if _, readErr := backend.ReadAt(buffer,
				layout.SourceExtents[0].VirtualOffset+offset); readErr != nil {
				b.Error(readErr)
				return
			}
		}
	})
	b.StopTimer()
	reportBackendMetrics(b, backend)
}
