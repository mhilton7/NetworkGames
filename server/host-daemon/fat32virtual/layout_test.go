package fat32virtual

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
	diskbackend "github.com/diskfs/go-diskfs/backend"
)

func testFile(t *testing.T, root, name string, size int) File {
	t.Helper()
	source := filepath.Join(root, name)
	data := make([]byte, size)
	for index := range data {
		data[index] = byte(index * 31)
	}
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	identity := Identity{
		Size: info.Size(), ModTimeUnixNano: info.ModTime().UnixNano(),
		SHA256: hex.EncodeToString(sum[:]),
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		identity.Device, identity.Inode = uint64(stat.Dev), stat.Ino
	}
	return File{
		VirtualPath: "/games/Long Synthetic Title [GTEST1]/game.iso",
		SourcePath:  source, LogicalSize: info.Size(), SourceSize: info.Size(),
		Identity: identity, GameID: "GTEST1", Format: "iso",
	}
}

func TestFAT32MetadataInspectableAndReadThrough(t *testing.T) {
	file := testFile(t, t.TempDir(), "source.iso", 2<<20)
	layout, metadata, err := Build(8<<30, "WIIBRIDGE", "stable", []File{file})
	if err != nil {
		t.Fatal(err)
	}
	backend, err := Open(layout, metadata, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	disk, err := diskfs.OpenBackend(&readStorage{backend: backend, size: backend.Size()})
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	fat, err := disk.GetFilesystem(1)
	if err != nil {
		t.Fatal(err)
	}
	defer fat.Close()
	if _, err = fat.Stat("games/Long Synthetic Title [GTEST1]/game.iso"); err != nil {
		t.Fatal(err)
	}
	actual := make([]byte, file.LogicalSize)
	if _, err = backend.ReadAt(actual, layout.SourceExtents[0].VirtualOffset); err != nil {
		t.Fatal(err)
	}
	expected, _ := os.ReadFile(file.SourcePath)
	if string(actual) != string(expected) {
		t.Fatal("read-through payload mismatch")
	}
}

func TestDuplicatePathsAndOverlappingExtentsRejected(t *testing.T) {
	file := testFile(t, t.TempDir(), "source.iso", 4096)
	duplicate := file
	duplicate.VirtualPath = "/GAMES/long synthetic title [gtest1]/GAME.ISO"
	if _, _, err := Build(8<<30, "WIIBRIDGE", "stable", []File{file, duplicate}); err == nil {
		t.Fatal("case-insensitive duplicate path accepted")
	}
	layout, metadata, err := Build(8<<30, "WIIBRIDGE", "stable", []File{file})
	if err != nil {
		t.Fatal(err)
	}
	layout.SourceExtents = append(layout.SourceExtents, layout.SourceExtents[0])
	if _, err = Open(layout, metadata, 2); err == nil {
		t.Fatal("overlapping source extents accepted")
	}
}

func BenchmarkReadManyExtents(b *testing.B) {
	root := b.TempDir()
	var files []File
	for index := 0; index < 1000; index++ {
		source := filepath.Join(root, fmt.Sprintf("source-%04d.bin", index))
		data := make([]byte, 512)
		if err := os.WriteFile(source, data, 0o600); err != nil {
			b.Fatal(err)
		}
		info, err := os.Stat(source)
		if err != nil {
			b.Fatal(err)
		}
		sum := sha256.Sum256(data)
		identity := Identity{
			Size: info.Size(), ModTimeUnixNano: info.ModTime().UnixNano(),
			SHA256: hex.EncodeToString(sum[:]),
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			identity.Device, identity.Inode = uint64(stat.Dev), stat.Ino
		}
		files = append(files, File{
			VirtualPath: fmt.Sprintf("/games/Title/file-%04d.bin", index),
			SourcePath:  source, LogicalSize: 512, SourceSize: 512,
			Identity: identity,
		})
	}
	layout, metadata, err := Build(8<<30, "WIIBRIDGE", "benchmark", files)
	if err != nil {
		b.Fatal(err)
	}
	backend, err := Open(layout, metadata, 16)
	if err != nil {
		b.Fatal(err)
	}
	defer backend.Close()
	buffer := make([]byte, 512)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		extent := layout.SourceExtents[index%len(layout.SourceExtents)]
		if _, err = backend.ReadAt(buffer, extent.VirtualOffset); err != nil {
			b.Fatal(err)
		}
	}
}

type readStorage struct {
	backend *Backend
	size    int64
	offset  int64
}

func (storage *readStorage) Read(buffer []byte) (int, error) {
	count, err := storage.backend.ReadAt(buffer, storage.offset)
	storage.offset += int64(count)
	return count, err
}
func (storage *readStorage) ReadAt(buffer []byte, offset int64) (int, error) {
	return storage.backend.ReadAt(buffer, offset)
}
func (storage *readStorage) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		storage.offset = offset
	case io.SeekCurrent:
		storage.offset += offset
	case io.SeekEnd:
		storage.offset = storage.size + offset
	default:
		return 0, errors.New("invalid whence")
	}
	return storage.offset, nil
}
func (storage *readStorage) Close() error { return nil }
func (storage *readStorage) Stat() (fs.FileInfo, error) {
	return virtualInfo{size: storage.size}, nil
}
func (storage *readStorage) Sys() (*os.File, error) { return nil, diskbackend.ErrNotSuitable }
func (storage *readStorage) Writable() (diskbackend.WritableFile, error) {
	return nil, diskbackend.ErrIncorrectOpenMode
}
func (storage *readStorage) Path() string { return "" }

type virtualInfo struct{ size int64 }

func (info virtualInfo) Name() string       { return "virtual.img" }
func (info virtualInfo) Size() int64        { return info.size }
func (info virtualInfo) Mode() fs.FileMode  { return 0o400 }
func (info virtualInfo) ModTime() time.Time { return time.Time{} }
func (info virtualInfo) IsDir() bool        { return false }
func (info virtualInfo) Sys() any           { return nil }
