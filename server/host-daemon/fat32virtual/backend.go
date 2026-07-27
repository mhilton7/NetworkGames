package fat32virtual

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sort"
	"sync"
	"syscall"
)

type cacheEntry struct {
	path string
	file *os.File
}

type Backend struct {
	mu       sync.Mutex
	size     int64
	metadata []byte
	meta     []MetadataExtent
	extents  []Extent
	limit    int
	cache    map[string]*list.Element
	lru      *list.List
	closed   bool
	stats    Stats
}

type Stats struct {
	SourceOpens   uint64
	SourceReads   uint64
	CacheHits     uint64
	PeakOpenFiles int
	CachedFiles   int
}

func Open(layout Layout, metadata []byte, cacheLimit int) (*Backend, error) {
	if layout.Schema != 2 || layout.VirtualSize <= 0 || cacheLimit < 1 {
		return nil, errors.New("invalid virtual FAT32 backend configuration")
	}
	for _, extent := range layout.MetadataExtents {
		if extent.StorageOffset < 0 || extent.Length < 0 ||
			extent.StorageOffset > int64(len(metadata))-extent.Length {
			return nil, errors.New("metadata extent exceeds metadata store")
		}
	}
	sum := sha256.Sum256(metadata)
	if hex.EncodeToString(sum[:]) != layout.MetadataHash ||
		hashExtents(layout.SourceExtents) != layout.ExtentMapHash {
		return nil, errors.New("virtual FAT32 metadata or extent-map hash mismatch")
	}
	if err := validateRanges(layout.VirtualSize, layout.MetadataExtents, layout.SourceExtents); err != nil {
		return nil, err
	}
	return &Backend{
		size: layout.VirtualSize, metadata: append([]byte(nil), metadata...),
		meta:    append([]MetadataExtent(nil), layout.MetadataExtents...),
		extents: append([]Extent(nil), layout.SourceExtents...),
		limit:   cacheLimit, cache: make(map[string]*list.Element), lru: list.New(),
	}, nil
}

func (b *Backend) Size() int64                        { return b.size }
func (b *Backend) ReadOnly() bool                     { return true }
func (b *Backend) WriteAt([]byte, int64) (int, error) { return 0, os.ErrPermission }
func (b *Backend) Sync() error                        { return nil }

func (b *Backend) Stats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := b.stats
	result.CachedFiles = len(b.cache)
	return result
}

func (b *Backend) ReadAt(buffer []byte, offset int64) (int, error) {
	if offset < 0 || int64(len(buffer)) > b.size-offset {
		return 0, io.EOF
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, os.ErrClosed
	}
	for done := 0; done < len(buffer); {
		position := offset + int64(done)
		if item, ok := findMetadata(b.meta, position); ok {
			count := len(buffer) - done
			available := item.VirtualOffset + item.Length - position
			if int64(count) > available {
				count = int(available)
			}
			source := item.StorageOffset + position - item.VirtualOffset
			copy(buffer[done:done+count], b.metadata[source:source+int64(count)])
			done += count
			continue
		}
		if item, ok := findSource(b.extents, position); ok {
			count := len(buffer) - done
			available := item.VirtualOffset + item.Length - position
			if int64(count) > available {
				count = int(available)
			}
			if err := verifyIdentity(item.SourcePath, item.Identity); err != nil {
				return done, err
			}
			file, err := b.open(item.SourcePath)
			if err != nil {
				return done, err
			}
			b.stats.SourceReads++
			read, readErr := readFullAt(file, buffer[done:done+count],
				item.SourceOffset+position-item.VirtualOffset)
			done += read
			if readErr != nil {
				return done, readErr
			}
			continue
		}
		next := offset + int64(len(buffer))
		if index := sort.Search(len(b.meta), func(index int) bool {
			return b.meta[index].VirtualOffset > position
		}); index < len(b.meta) && b.meta[index].VirtualOffset < next {
			next = b.meta[index].VirtualOffset
		}
		if index := sort.Search(len(b.extents), func(index int) bool {
			return b.extents[index].VirtualOffset > position
		}); index < len(b.extents) && b.extents[index].VirtualOffset < next {
			next = b.extents[index].VirtualOffset
		}
		count := int(next - position)
		if count < 1 {
			count = 1
		}
		clear(buffer[done : done+count])
		done += count
	}
	return len(buffer), nil
}

func findMetadata(items []MetadataExtent, position int64) (MetadataExtent, bool) {
	index := sort.Search(len(items), func(index int) bool {
		return items[index].VirtualOffset+items[index].Length > position
	})
	if index < len(items) && position >= items[index].VirtualOffset {
		return items[index], true
	}
	return MetadataExtent{}, false
}

func findSource(items []Extent, position int64) (Extent, bool) {
	index := sort.Search(len(items), func(index int) bool {
		return items[index].VirtualOffset+items[index].Length > position
	})
	if index < len(items) && position >= items[index].VirtualOffset {
		return items[index], true
	}
	return Extent{}, false
}

func verifyIdentity(path string, expected Identity) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() != expected.Size || info.ModTime().UnixNano() != expected.ModTimeUnixNano {
		return errors.New("GameCube source identity changed while active")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok &&
		(uint64(stat.Dev) != expected.Device || stat.Ino != expected.Inode) {
		return errors.New("GameCube source file was replaced while active")
	}
	return nil
}

func (b *Backend) open(path string) (*os.File, error) {
	if element := b.cache[path]; element != nil {
		b.stats.CacheHits++
		b.lru.MoveToFront(element)
		return element.Value.(*cacheEntry).file, nil
	}
	if b.lru.Len() >= b.limit {
		oldest := b.lru.Back()
		entry := oldest.Value.(*cacheEntry)
		delete(b.cache, entry.path)
		b.lru.Remove(oldest)
		if err := entry.file.Close(); err != nil {
			return nil, err
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	b.stats.SourceOpens++
	element := b.lru.PushFront(&cacheEntry{path: path, file: file})
	b.cache[path] = element
	if len(b.cache) > b.stats.PeakOpenFiles {
		b.stats.PeakOpenFiles = len(b.cache)
	}
	return file, nil
}

func (b *Backend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	var first error
	for _, element := range b.cache {
		if err := element.Value.(*cacheEntry).file.Close(); err != nil && first == nil {
			first = err
		}
	}
	b.cache = nil
	b.lru.Init()
	return first
}

func readFullAt(reader io.ReaderAt, buffer []byte, offset int64) (int, error) {
	done := 0
	for done < len(buffer) {
		count, err := reader.ReadAt(buffer[done:], offset+int64(done))
		done += count
		if done == len(buffer) {
			return done, nil
		}
		if err != nil {
			return done, err
		}
		if count == 0 {
			return done, io.ErrNoProgress
		}
	}
	return done, nil
}
