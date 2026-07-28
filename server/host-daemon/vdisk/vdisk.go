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

	"wiibridge/server/host-daemon/scanner"
	"wiibridge/shared/model"
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

type clusterChain struct {
	first, count uint32
}

type Disk struct {
	size       int64
	metadata   map[int64][]byte
	fatStart   int64
	fatSectors int64
	fatChains  []clusterChain
	extents    []extent
	snapshot   model.Snapshot
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
	signatureHash := sha256.New()
	signatureHash.Write([]byte(catalog))
	for _, file := range files {
		fmt.Fprintf(signatureHash, "\x00%s\x00%d\x00%d", file.name, file.off, file.length)
	}
	signatureBytes := signatureHash.Sum(nil)
	diskSignature := binary.LittleEndian.Uint32(signatureBytes[:4])
	if diskSignature == 0 {
		// Zero means that an MBR disk has no identity. It is particularly
		// unsuitable for this fixed, read-only USB LUN because Windows cannot
		// assign and persist a replacement signature.
		diskSignature = 0x57424d53
	}
	d := &Disk{size: diskSectors * sectorSize, metadata: map[int64][]byte{}}
	d.metadata[0] = mbr(partitionStart, partSectors, diskSignature)
	boot := bootSector(partSectors, fatSectors, uint32(2), diskSignature)
	d.metadata[partitionStart] = boot
	d.metadata[partitionStart+6] = append([]byte(nil), boot...)
	d.metadata[partitionStart+1] = fsInfo()
	d.metadata[partitionStart+7] = fsInfo()
	nextCluster := uint32(2)
	chain := func(count int64) uint32 {
		first := nextCluster
		if count > 0 {
			d.fatChains = append(d.fatChains, clusterChain{
				first: first, count: uint32(count),
			})
		}
		nextCluster += uint32(count)
		return first
	}
	d.fatStart = partitionStart + reservedSectors
	d.fatSectors = fatSectors
	chain(rootClusters) // FAT32 root is fixed at cluster 2.
	wbfsCluster := chain(wbfsDirClusters)
	for i := range files {
		files[i].first = chain((files[i].length + clusterSize - 1) / clusterSize)
	}
	dataStartSector := partitionStart + firstData
	rootDir := make([]byte, rootClusters*clusterSize)
	write83(rootDir[0:32], "WIIBRIDGE", 0, 0, 0x08)
	write83(rootDir[32:64], "WBFS       ", wbfsCluster, 0, 0x10)
	dir := make([]byte, wbfsDirClusters*clusterSize)
	write83(dir[0:32], ".", wbfsCluster, 0, 0x10)
	write83(dir[32:64], "..", 0, 0, 0x10)
	for i, f := range files {
		base := (2 + i*2) * 32
		writeLFN(dir[base:base+32], f.name, lfnChecksum([]byte(f.short)))
		// USB Loader GX opens WBFS files with O_RDWR even when it only needs
		// to extract a banner or boot a game. A DOS read-only attribute makes
		// that open fail before any payload read. Immutability is enforced by
		// the read-only NBD export and USB LUN, not by the FAT entry.
		write83(dir[base+32:base+64], f.short, f.first, uint32(f.length), 0x20)
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
	metaHash := d.metadataIdentity()
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

// metadataIdentity fingerprints the compact description used to synthesize the
// disk. Hashing every apparent FAT sector would make Host startup proportional
// to virtual library capacity even though those sectors are generated on
// demand and consume no resident storage.
func (d *Disk) metadataIdentity() string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("wiibridge-vdisk-compact-metadata-v1\x00"))
	var number [8]byte
	writeInt64 := func(value int64) {
		binary.LittleEndian.PutUint64(number[:], uint64(value))
		_, _ = hash.Write(number[:])
	}
	writeUint32 := func(value uint32) {
		binary.LittleEndian.PutUint32(number[:4], value)
		_, _ = hash.Write(number[:4])
	}
	writeInt64(d.size)
	writeInt64(sectorSize)
	writeInt64(sectorsPerCluster)
	writeInt64(d.fatStart)
	writeInt64(d.fatSectors)
	writeInt64(numFATs)

	keys := make([]int64, 0, len(d.metadata))
	for sector := range d.metadata {
		keys = append(keys, sector)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	writeInt64(int64(len(keys)))
	for _, sector := range keys {
		data := d.metadata[sector]
		writeInt64(sector)
		writeInt64(int64(len(data)))
		_, _ = hash.Write(data)
	}

	writeInt64(int64(len(d.fatChains)))
	for _, chain := range d.fatChains {
		writeUint32(chain.first)
		writeUint32(chain.count)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (d *Disk) fatSectorIndex(sector int64) (int64, bool) {
	relative := sector - d.fatStart
	if relative >= 0 && relative < d.fatSectors {
		return relative, true
	}
	relative -= d.fatSectors
	return relative, relative >= 0 && relative < d.fatSectors
}

func (d *Disk) fatValue(cluster uint32) uint32 {
	switch cluster {
	case 0:
		return 0x0ffffff8
	case 1:
		return 0xffffffff
	}
	index := sort.Search(len(d.fatChains), func(index int) bool {
		chain := d.fatChains[index]
		return chain.first+chain.count > cluster
	})
	if index == len(d.fatChains) {
		return 0
	}
	chain := d.fatChains[index]
	if cluster < chain.first {
		return 0
	}
	if cluster+1 == chain.first+chain.count {
		return 0x0fffffff
	}
	return cluster + 1
}

func (d *Disk) fillFATSector(target []byte, fatSector int64) {
	clear(target)
	firstCluster := uint32(fatSector * (sectorSize / 4))
	for offset := 0; offset < len(target); offset += 4 {
		binary.LittleEndian.PutUint32(target[offset:offset+4],
			d.fatValue(firstCluster+uint32(offset/4)))
	}
}

func (d *Disk) metadataSector(sector int64) ([]byte, bool) {
	if data, ok := d.metadata[sector]; ok {
		return data, true
	}
	fatSector, ok := d.fatSectorIndex(sector)
	if !ok {
		return nil, false
	}
	data := make([]byte, sectorSize)
	d.fillFATSector(data, fatSector)
	return data, true
}

func (d *Disk) forEachMetadataSector(visit func(int64, []byte) error) error {
	type region struct {
		start, count int64
		fat          bool
		data         []byte
	}
	regions := make([]region, 0, len(d.metadata)+int(numFATs))
	for sector, data := range d.metadata {
		regions = append(regions, region{start: sector, count: 1, data: data})
	}
	for copyNumber := int64(0); copyNumber < numFATs; copyNumber++ {
		regions = append(regions, region{
			start: d.fatStart + copyNumber*d.fatSectors,
			count: d.fatSectors, fat: true,
		})
	}
	sort.Slice(regions, func(i, j int) bool { return regions[i].start < regions[j].start })
	buffer := make([]byte, sectorSize)
	for _, item := range regions {
		for index := int64(0); index < item.count; index++ {
			data := item.data
			if item.fat {
				d.fillFATSector(buffer, index)
				data = buffer
			}
			if err := visit(item.start+index, data); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Disk) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || int64(len(p)) > d.size-off {
		return 0, io.EOF
	}
	for done := 0; done < len(p); {
		pos := off + int64(done)
		sector := pos / sectorSize
		inSector := pos % sectorSize
		n := len(p) - done
		if data, ok := d.metadataSector(sector); ok {
			if max := int(sectorSize - inSector); n > max {
				n = max
			}
			copy(p[done:done+n], data[inSector:inSector+int64(n)])
		} else {
			var mapped *extent
			for _, e := range d.extents {
				if pos >= e.start && pos < e.start+e.length {
					mapped = &e
					break
				}
			}
			if mapped == nil {
				// Metadata occupies whole sectors, so an unmapped gap can be
				// cleared through the current sector without hiding metadata.
				if max := int(sectorSize - inSector); n > max {
					n = max
				}
				clear(p[done : done+n])
			} else {
				if available := mapped.start + mapped.length - pos; int64(n) > available {
					n = int(available)
				}
				if mapped.zero {
					clear(p[done : done+n])
				} else {
					if err := scanner.VerifySource(*mapped.source); err != nil {
						return done, err
					}
					f, err := os.Open(mapped.source.Path)
					if err != nil {
						return done, err
					}
					readN, readErr := readAtFull(f, p[done:done+n],
						mapped.sourceOffset+pos-mapped.start)
					closeErr := f.Close()
					if readErr != nil {
						return done + readN, readErr
					}
					if closeErr != nil {
						return done + readN, closeErr
					}
				}
			}
		}
		done += n
	}
	return len(p), nil
}

func readAtFull(r io.ReaderAt, p []byte, off int64) (int, error) {
	done := 0
	for done < len(p) {
		n, err := r.ReadAt(p[done:], off+int64(done))
		done += n
		if done == len(p) {
			return done, nil
		}
		if err != nil {
			return done, err
		}
		if n == 0 {
			return done, io.ErrNoProgress
		}
	}
	return done, nil
}

func mbr(start, sectors int64, signature uint32) []byte {
	b := make([]byte, 512)
	binary.LittleEndian.PutUint32(b[440:444], signature)
	p := b[446:462]
	p[0], p[4] = 0x00, 0x0c
	p[1], p[2], p[3] = 0x20, 0x21, 0x00
	p[5], p[6], p[7] = 0xfe, 0xff, 0xff
	binary.LittleEndian.PutUint32(p[8:12], uint32(start))
	binary.LittleEndian.PutUint32(p[12:16], uint32(sectors))
	b[510], b[511] = 0x55, 0xaa
	return b
}

func bootSector(sectors, fatSectors int64, root, volumeID uint32) []byte {
	b := make([]byte, 512)
	copy(b[0:3], []byte{0xeb, 0x58, 0x90})
	copy(b[3:11], "WIIBRDG ")
	binary.LittleEndian.PutUint16(b[11:13], 512)
	b[13] = byte(sectorsPerCluster)
	binary.LittleEndian.PutUint16(b[14:16], uint16(reservedSectors))
	b[16] = byte(numFATs)
	b[21] = 0xf8
	// Use conventional LBA-assisted disk geometry. Modern hosts address this
	// image by LBA, but Windows and mtools still reject a FAT BPB whose legacy
	// geometry and hidden-sector fields are left at zero.
	binary.LittleEndian.PutUint16(b[24:26], 63)
	binary.LittleEndian.PutUint16(b[26:28], 255)
	binary.LittleEndian.PutUint32(b[28:32], uint32(partitionStart))
	binary.LittleEndian.PutUint32(b[32:36], uint32(sectors))
	binary.LittleEndian.PutUint32(b[36:40], uint32(fatSectors))
	binary.LittleEndian.PutUint32(b[44:48], root)
	binary.LittleEndian.PutUint16(b[48:50], 1)
	binary.LittleEndian.PutUint16(b[50:52], 6)
	b[64], b[66] = 0x80, 0x29
	binary.LittleEndian.PutUint32(b[67:71], volumeID)
	copy(b[71:82], "WIIBRIDGE  ")
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
	_ = d.forEachMetadataSector(func(_ int64, data []byte) error {
		_, _ = out.Write(data)
		return nil
	})
	return out.Bytes()
}
