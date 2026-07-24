// Package vdisk synthesizes immutable MBR/FAT32 metadata and maps data clusters
// directly to read-only source ranges.
package vdisk

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"networkgames/server/host-daemon/scanner"
	"networkgames/shared/model"
)

const (
	sectorSize        = int64(512)
	sectorsPerCluster = int64(8)
	clusterSize       = sectorSize * sectorsPerCluster
	partitionStart    = int64(2048)
	reservedSectors   = int64(32)
	numFATs           = int64(2)
	maxSegment        = int64(0xfffff000)
)

type extent struct {
	start, length int64
	source        *model.Source
	sourceOffset  int64
	zero          bool
}

type Disk struct {
	size     int64
	metadata map[int64][]byte
	extents  []extent
	snapshot model.Snapshot
}

func Build(catalog string, games []model.Game, appVersion string) (*Disk, error) {
	if catalog == "" {
		return nil, errors.New("catalog ID required")
	}
	games = append([]model.Game(nil), games...)
	sort.Slice(games, func(i, j int) bool { return games[i].ID < games[j].ID })
	type virtualFile struct {
		name, short string
		game        model.Game
		off, length int64
		first       uint32
	}
	var files []virtualFile
	var dataClusters int64
	for _, g := range games {
		for off, seg := int64(0), 0; off < g.Size; seg++ {
			n := g.Size - off
			if n > maxSegment {
				n = maxSegment
			}
			ext := "WBFS"
			if seg > 0 {
				ext = fmt.Sprintf("WBF%d", seg)
			}
			longName := fmt.Sprintf("%s.%s", g.ID, strings.ToLower(ext))
			sum := sha256.Sum256([]byte(longName))
			short := fmt.Sprintf("%-8s%-3s", fmt.Sprintf("%.2s%05X", g.ID, sum[:3]), "WBF")
			files = append(files, virtualFile{name: longName, short: short, game: g, off: off, length: n})
			dataClusters += (n + clusterSize - 1) / clusterSize
			off += n
		}
	}
	// Each payload uses one VFAT long-name entry plus one unique 8.3 alias.
	wbfsEntries := int64(2 + 2*len(files))
	wbfsDirClusters := (wbfsEntries*32 + clusterSize - 1) / clusterSize
	if wbfsDirClusters < 1 {
		wbfsDirClusters = 1
	}
	const rootClusters = int64(1)
	totalClusters := rootClusters + wbfsDirClusters + dataClusters
	// FAT32 requires at least 65,525 clusters. Unallocated clusters are virtual
	// zero space and do not consume persistent storage.
	if totalClusters < 65525 {
		totalClusters = 65525
	}
	fatSectors := ((totalClusters+2)*4 + sectorSize - 1) / sectorSize
	firstData := reservedSectors + numFATs*fatSectors
	partSectors := firstData + totalClusters*sectorsPerCluster
	diskSectors := partitionStart + partSectors
	d := &Disk{size: diskSectors * sectorSize, metadata: map[int64][]byte{}}
	d.metadata[0] = mbr(partitionStart, partSectors)
	boot := bootSector(partSectors, fatSectors, uint32(2))
	d.metadata[partitionStart] = boot
	d.metadata[partitionStart+6] = append([]byte(nil), boot...)
	d.metadata[partitionStart+1] = fsInfo()
	d.metadata[partitionStart+7] = fsInfo()
	fat := make([]byte, fatSectors*sectorSize)
	binary.LittleEndian.PutUint32(fat[0:4], 0x0ffffff8)
	binary.LittleEndian.PutUint32(fat[4:8], 0xffffffff)
	nextCluster := uint32(2)
	chain := func(count int64) uint32 {
		first := nextCluster
		for i := int64(0); i < count; i++ {
			value := uint32(0x0fffffff)
			if i+1 < count {
				value = nextCluster + 1
			}
			binary.LittleEndian.PutUint32(fat[int(nextCluster)*4:], value)
			nextCluster++
		}
		return first
	}
	chain(rootClusters) // FAT32 root is fixed at cluster 2.
	wbfsCluster := chain(wbfsDirClusters)
	for i := range files {
		files[i].first = chain((files[i].length + clusterSize - 1) / clusterSize)
	}
	for copyNo := int64(0); copyNo < numFATs; copyNo++ {
		base := partitionStart + reservedSectors + copyNo*fatSectors
		for i := int64(0); i < fatSectors; i++ {
			d.metadata[base+i] = append([]byte(nil), fat[i*sectorSize:(i+1)*sectorSize]...)
		}
	}
	dataStartSector := partitionStart + firstData
	rootDir := make([]byte, rootClusters*clusterSize)
	write83(rootDir[0:32], "NETWORKGAME", 0, 0, 0x08)
	write83(rootDir[32:64], "WBFS       ", wbfsCluster, 0, 0x10)
	dir := make([]byte, wbfsDirClusters*clusterSize)
	write83(dir[0:32], ".", wbfsCluster, 0, 0x10)
	write83(dir[32:64], "..", 0, 0, 0x10)
	for i, f := range files {
		base := (2 + i*2) * 32
		writeLFN(dir[base:base+32], f.name, lfnChecksum([]byte(f.short)))
		write83(dir[base+32:base+64], f.short, f.first, uint32(f.length), 0x01)
	}
	for i := int64(0); i < rootClusters*sectorsPerCluster; i++ {
		d.metadata[dataStartSector+i] = append([]byte(nil), rootDir[i*sectorSize:(i+1)*sectorSize]...)
	}
	wbfsStartSector := dataStartSector + rootClusters*sectorsPerCluster
	for i := int64(0); i < wbfsDirClusters*sectorsPerCluster; i++ {
		d.metadata[wbfsStartSector+i] = append([]byte(nil), dir[i*sectorSize:(i+1)*sectorSize]...)
	}
	cursor := wbfsStartSector + wbfsDirClusters*sectorsPerCluster
	for _, vf := range files {
		fileStart := cursor * sectorSize
		remaining := vf.length
		gameOffset := vf.off
		for remaining > 0 {
			source, sourceOff, err := locate(vf.game.Sources, gameOffset)
			if err != nil {
				return nil, err
			}
			n := remaining
			available := source.Length - sourceOff
			if n > available {
				n = available
			}
			d.extents = append(d.extents, extent{
				start: fileStart + gameOffset - vf.off, length: n, source: source, sourceOffset: sourceOff,
			})
			gameOffset += n
			remaining -= n
		}
		padding := ((vf.length+clusterSize-1)/clusterSize)*clusterSize - vf.length
		if padding > 0 {
			d.extents = append(d.extents, extent{start: fileStart + vf.length, length: padding, zero: true})
		}
		cursor += ((vf.length + clusterSize - 1) / clusterSize) * sectorsPerCluster
	}
	hash := sha256.New()
	keys := make([]int64, 0, len(d.metadata))
	for k := range d.metadata {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		hash.Write(d.metadata[k])
	}
	metaHash := hex.EncodeToString(hash.Sum(nil))
	var virtualFiles []model.VirtualFile
	for _, file := range files {
		virtualFiles = append(virtualFiles, model.VirtualFile{
			Path: "/wbfs/" + file.name, GameID: file.game.ID,
			LogicalStart: file.off, Length: file.length,
		})
	}
	identityBytes, err := json.Marshal(struct {
		Games []model.Game
		Files []model.VirtualFile
	}{games, virtualFiles})
	if err != nil {
		return nil, err
	}
	idMaterial := append([]byte(catalog+"\x00"+metaHash+"\x00"), identityBytes...)
	idSum := sha256.Sum256(idMaterial)
	d.snapshot = model.Snapshot{
		SnapshotID: hex.EncodeToString(idSum[:16]), CatalogID: catalog,
		VirtualDiskSize: d.size, MetadataHash: metaHash, Application: appVersion,
		Created: time.Now().UTC(), Games: games, VirtualFiles: virtualFiles,
	}
	return d, nil
}

func locate(sources []model.Source, logical int64) (*model.Source, int64, error) {
	for i := range sources {
		if logical >= sources[i].Offset && logical < sources[i].Offset+sources[i].Length {
			return &sources[i], logical - sources[i].Offset, nil
		}
	}
	return nil, 0, errors.New("source extent gap")
}

func (d *Disk) Size() int64              { return d.size }
func (d *Disk) Snapshot() model.Snapshot { return d.snapshot }

func (d *Disk) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || int64(len(p)) > d.size-off {
		return 0, io.EOF
	}
	for done := 0; done < len(p); {
		pos := off + int64(done)
		sector := pos / sectorSize
		inSector := pos % sectorSize
		n := len(p) - done
		if max := int(sectorSize - inSector); n > max {
			n = max
		}
		if data, ok := d.metadata[sector]; ok {
			copy(p[done:done+n], data[inSector:inSector+int64(n)])
		} else {
			for i := done; i < done+n; i++ {
				p[i] = 0
			}
			for _, e := range d.extents {
				if pos >= e.start && pos < e.start+e.length {
					if !e.zero {
						if err := scanner.VerifySource(*e.source); err != nil {
							return done, err
						}
						f, err := os.Open(e.source.Path)
						if err != nil {
							return done, err
						}
						readN, readErr := f.ReadAt(p[done:done+n], e.sourceOffset+pos-e.start)
						f.Close()
						if readErr != nil && readErr != io.EOF {
							return done + readN, readErr
						}
					}
					break
				}
			}
		}
		done += n
	}
	return len(p), nil
}

func mbr(start, sectors int64) []byte {
	b := make([]byte, 512)
	p := b[446:462]
	p[0], p[4] = 0x00, 0x0c
	p[1], p[2], p[3] = 0x20, 0x21, 0x00
	p[5], p[6], p[7] = 0xfe, 0xff, 0xff
	binary.LittleEndian.PutUint32(p[8:12], uint32(start))
	binary.LittleEndian.PutUint32(p[12:16], uint32(sectors))
	b[510], b[511] = 0x55, 0xaa
	return b
}

func bootSector(sectors, fatSectors int64, root uint32) []byte {
	b := make([]byte, 512)
	copy(b[0:3], []byte{0xeb, 0x58, 0x90})
	copy(b[3:11], "NGAMES  ")
	binary.LittleEndian.PutUint16(b[11:13], 512)
	b[13] = byte(sectorsPerCluster)
	binary.LittleEndian.PutUint16(b[14:16], uint16(reservedSectors))
	b[16] = byte(numFATs)
	b[21] = 0xf8
	binary.LittleEndian.PutUint32(b[32:36], uint32(sectors))
	binary.LittleEndian.PutUint32(b[36:40], uint32(fatSectors))
	binary.LittleEndian.PutUint32(b[44:48], root)
	binary.LittleEndian.PutUint16(b[48:50], 1)
	binary.LittleEndian.PutUint16(b[50:52], 6)
	b[64], b[66], b[67] = 0x80, 0x29, 0x42
	copy(b[71:82], "NETWORKGAMES")
	copy(b[82:90], "FAT32   ")
	b[510], b[511] = 0x55, 0xaa
	return b
}

func fsInfo() []byte {
	b := make([]byte, 512)
	copy(b[0:4], []byte("RRaA"))
	copy(b[484:488], []byte("rrAa"))
	binary.LittleEndian.PutUint32(b[488:492], 0xffffffff)
	binary.LittleEndian.PutUint32(b[492:496], 0xffffffff)
	b[510], b[511] = 0x55, 0xaa
	return b
}

func write83(dst []byte, name string, cluster uint32, size uint32, attr byte) {
	for i := range dst {
		dst[i] = 0
	}
	for i := 0; i < 11; i++ {
		dst[i] = ' '
	}
	if name == "." || name == ".." {
		copy(dst[:], name)
	} else {
		copy(dst[:11], strings.ToUpper(name))
	}
	dst[11] = attr
	binary.LittleEndian.PutUint16(dst[20:22], uint16(cluster>>16))
	binary.LittleEndian.PutUint16(dst[26:28], uint16(cluster))
	binary.LittleEndian.PutUint32(dst[28:32], size)
}

func lfnChecksum(short []byte) byte {
	var sum byte
	for _, c := range short[:11] {
		sum = ((sum & 1) << 7) + (sum >> 1) + c
	}
	return sum
}

func writeLFN(dst []byte, name string, checksum byte) {
	for i := range dst {
		dst[i] = 0xff
	}
	dst[0] = 0x41 // final and first entry in this one-entry LFN
	dst[11] = 0x0f
	dst[12] = 0
	dst[13] = checksum
	dst[26], dst[27] = 0, 0
	positions := []int{1, 3, 5, 7, 9, 14, 16, 18, 20, 22, 24, 28, 30}
	runes := []rune(name)
	for i, pos := range positions {
		var value uint16 = 0xffff
		if i < len(runes) {
			value = uint16(runes[i])
		} else if i == len(runes) {
			value = 0
		}
		binary.LittleEndian.PutUint16(dst[pos:pos+2], value)
	}
}

func (d *Disk) MetadataBytes() []byte {
	var out bytes.Buffer
	keys := make([]int64, 0, len(d.metadata))
	for k := range d.metadata {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		out.Write(d.metadata[k])
	}
	return out.Bytes()
}
