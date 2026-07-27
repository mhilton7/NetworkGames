package vdisk

import (
	"os"
	"path/filepath"
	"testing"

	"wiibridge/server/host-daemon/scanner"
	"wiibridge/tests/testutil"
)

func BenchmarkPayloadRead1MiB(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "PERF01.wbfs")
	if err := testutil.SyntheticWBFS(path, "PERF01", "Performance", 64<<20); err != nil {
		b.Fatal(err)
	}
	// Allocate the otherwise sparse synthetic fixture so this benchmark
	// measures the virtual-disk read path rather than sparse-hole behavior.
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	if err = f.Truncate(0); err != nil {
		f.Close()
		b.Fatal(err)
	}
	block := make([]byte, 1<<20)
	for off := int64(0); off < 64<<20; off += int64(len(block)) {
		if _, err = f.Write(block); err != nil {
			f.Close()
			b.Fatal(err)
		}
	}
	if err = f.Close(); err != nil {
		b.Fatal(err)
	}
	// Restore a valid synthetic header after allocating the complete file.
	if err = testutil.SyntheticWBFS(path, "PERF01", "Performance", 64<<20); err != nil {
		b.Fatal(err)
	}
	scan, err := scanner.Scan(root)
	if err != nil {
		b.Fatal(err)
	}
	disk, err := Build("performance", scan.Games, "benchmark")
	if err != nil {
		b.Fatal(err)
	}
	if len(disk.extents) == 0 || disk.extents[0].zero {
		b.Fatal("payload extent missing")
	}
	buf := make([]byte, 1<<20)
	start := disk.extents[0].start
	b.SetBytes(int64(len(buf)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = disk.ReadAt(buf, start); err != nil {
			b.Fatal(err)
		}
	}
}
