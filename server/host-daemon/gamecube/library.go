package gamecube

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
	diskpkg "github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/partition/mbr"
)

const (
	LibrarySchema          = 1
	libraryMinimumSize     = int64(8 << 30)
	libraryAlignment       = int64(1 << 30)
	libraryMetadataReserve = int64(256 << 20)
	fat32MaximumFileSize   = int64(1<<32 - 1)
)

type LibraryConfig struct {
	HeadroomPercent int
	SaveReserveMiB  int64
	MaxVolumeGiB    int64
	Mode            MemoryCardMode
	Retention       int
}

func DefaultLibraryConfig() LibraryConfig {
	return LibraryConfig{
		HeadroomPercent: 5, SaveReserveMiB: 1024,
		Mode: MemoryCardPhysical, Retention: 2,
	}
}

func (c LibraryConfig) Validate() error {
	if c.HeadroomPercent < 0 || c.HeadroomPercent > 100 {
		return errors.New("GameCube headroom percent must be between 0 and 100")
	}
	if c.SaveReserveMiB < 0 || c.SaveReserveMiB > math.MaxInt64>>20 {
		return errors.New("invalid GameCube save reserve")
	}
	if c.MaxVolumeGiB < 0 || c.MaxVolumeGiB > math.MaxInt64>>30 {
		return errors.New("invalid GameCube maximum volume")
	}
	if c.Mode != MemoryCardPhysical && c.Mode != MemoryCardEmulated {
		return errors.New("invalid GameCube library memory-card mode")
	}
	if c.Retention < 1 || c.Retention > 20 {
		return errors.New("GameCube generation retention must be between 1 and 20")
	}
	return nil
}

type LibraryTitle struct {
	Title      string `json:"title"`
	ID         string `json:"game_id"`
	Revision   byte   `json:"revision"`
	Region     string `json:"region"`
	Format     string `json:"source_format"`
	DiscCount  int    `json:"disc_count"`
	OutputDir  string `json:"output_directory"`
	PreparedID string `json:"prepared_artifact_identity"`
}

type LibraryManifest struct {
	Schema             int            `json:"schema"`
	GenerationID       string         `json:"generation_id"`
	Created            time.Time      `json:"created_utc"`
	ImagePath          string         `json:"image_path"`
	VolumeSize         int64          `json:"virtual_image_size"`
	Filesystem         string         `json:"filesystem"`
	ClusterSize        int            `json:"cluster_size"`
	Mode               MemoryCardMode `json:"memory_card_mode"`
	ReadOnly           bool           `json:"read_only"`
	TitleCount         int            `json:"total_title_count"`
	DiscCount          int            `json:"total_disc_count"`
	CatalogFingerprint string         `json:"source_catalog_fingerprint"`
	Titles             []LibraryTitle `json:"titles"`
	Complete           bool           `json:"complete"`
}

type LibraryBuildProgress struct {
	State          string    `json:"state"`
	GamesCompleted int       `json:"games_completed"`
	TotalGames     int       `json:"total_games"`
	DiscsCompleted int       `json:"discs_completed"`
	TotalDiscs     int       `json:"total_discs"`
	CurrentTitle   string    `json:"current_title,omitempty"`
	BytesWritten   int64     `json:"bytes_written"`
	Started        time.Time `json:"started,omitempty"`
	Completed      time.Time `json:"completed,omitempty"`
	Error          string    `json:"error,omitempty"`
}

type LibraryManager struct {
	mu       sync.RWMutex
	root     string
	config   LibraryConfig
	progress LibraryBuildProgress
	cancel   context.CancelFunc
}

func NewLibraryManager(root string, config LibraryConfig) (*LibraryManager, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	clean, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Join(clean, "generations"), 0o700); err != nil {
		return nil, err
	}
	manager := &LibraryManager{root: clean, config: config,
		progress: LibraryBuildProgress{State: "Not built"}}
	entries, err := os.ReadDir(filepath.Join(clean, "generations"))
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".building-") {
			_ = os.RemoveAll(filepath.Join(clean, "generations", entry.Name()))
		}
	}
	if active, activeErr := manager.Active(); activeErr == nil {
		manager.progress = LibraryBuildProgress{
			State: "Ready", GamesCompleted: active.TitleCount,
			TotalGames: active.TitleCount, DiscsCompleted: active.DiscCount,
			TotalDiscs: active.DiscCount, Completed: active.Created,
		}
		_ = manager.prune(active.GenerationID)
	}
	return manager, nil
}

func (m *LibraryManager) Root() string { return m.root }

func (m *LibraryManager) Progress() LibraryBuildProgress {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.progress
}

func (m *LibraryManager) Cancel() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel == nil {
		return false
	}
	m.cancel()
	return true
}

func (m *LibraryManager) StartBuild(ctx context.Context, games []Game) error {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return errors.New("GameCube library build is already running")
	}
	buildCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	totalDiscs := 0
	for _, game := range games {
		totalDiscs += len(game.Discs)
	}
	m.progress = LibraryBuildProgress{
		State: "Building", TotalGames: len(games), TotalDiscs: totalDiscs,
		Started: time.Now().UTC(),
	}
	m.mu.Unlock()
	go func() {
		manifest, err := m.build(buildCtx, games)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cancel = nil
		m.progress.Completed = time.Now().UTC()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				m.progress.State = "Not built"
				m.progress.Error = "build canceled"
			} else {
				m.progress.State = "Failed"
				m.progress.Error = boundedError(err)
			}
			return
		}
		m.progress.State = "Ready"
		m.progress.GamesCompleted = manifest.TitleCount
		m.progress.DiscsCompleted = manifest.DiscCount
		m.progress.CurrentTitle = ""
	}()
	return nil
}

func boundedError(err error) string {
	value := err.Error()
	for _, safe := range []string{
		"requires", "capacity", "overflow", "no validated GameCube",
		"FAT32", "build canceled",
	} {
		if strings.Contains(value, safe) {
			if len(value) > 240 {
				return value[:240]
			}
			return value
		}
	}
	return "GameCube library build failed during source preparation or volume generation"
}

func (m *LibraryManager) setCurrent(title string, games, discs int, bytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress.CurrentTitle = title
	m.progress.GamesCompleted = games
	m.progress.DiscsCompleted = discs
	m.progress.BytesWritten = bytes
}

func (m *LibraryManager) Build(ctx context.Context, games []Game) (LibraryManifest, error) {
	return m.build(ctx, games)
}

func (m *LibraryManager) build(ctx context.Context, games []Game) (LibraryManifest, error) {
	if len(games) == 0 {
		return LibraryManifest{}, errors.New("no validated GameCube titles are available")
	}
	hashed := make([]Game, len(games))
	var payloadBytes int64
	totalDiscs := 0
	for index, game := range games {
		if err := ctx.Err(); err != nil {
			return LibraryManifest{}, err
		}
		if game.Validation != "valid" || len(game.Discs) == 0 || len(game.Discs) > 2 {
			return LibraryManifest{}, fmt.Errorf("%s is not a validated one- or two-disc title", game.ID)
		}
		var err error
		hashed[index], err = hashGameSources(game)
		if err != nil {
			return LibraryManifest{}, err
		}
		for _, disc := range hashed[index].Discs {
			if disc.PhysicalSize > fat32MaximumFileSize && disc.Format != "fst" {
				return LibraryManifest{}, fmt.Errorf("%s disc %d exceeds the FAT32 file limit", game.ID, disc.Number+1)
			}
			if disc.Format == "fst" {
				if err := validateFSTFileSizes(disc.SourcePath); err != nil {
					return LibraryManifest{}, err
				}
			}
			if disc.PhysicalSize < 0 || payloadBytes > math.MaxInt64-disc.PhysicalSize {
				return LibraryManifest{}, errors.New("GameCube library size overflow")
			}
			payloadBytes += disc.PhysicalSize
			totalDiscs++
		}
	}
	sort.Slice(hashed, func(i, j int) bool {
		if hashed[i].ID == hashed[j].ID {
			return hashed[i].Revision < hashed[j].Revision
		}
		return hashed[i].ID < hashed[j].ID
	})
	fingerprint := catalogFingerprint(hashed)
	size, err := CalculateLibrarySize(payloadBytes, m.config)
	if err != nil {
		return LibraryManifest{}, err
	}
	generationID := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + fingerprint[:16]
	generations := filepath.Join(m.root, "generations")
	staging := filepath.Join(generations, ".building-"+generationID)
	final := filepath.Join(generations, generationID)
	if err = os.Mkdir(staging, 0o700); err != nil {
		return LibraryManifest{}, err
	}
	defer os.RemoveAll(staging)
	imagePath := filepath.Join(staging, "library.img")
	manifest := LibraryManifest{
		Schema: LibrarySchema, GenerationID: generationID, Created: time.Now().UTC(),
		ImagePath: filepath.Join(final, "library.img"), VolumeSize: size,
		Filesystem: "fat32", Mode: m.config.Mode,
		ReadOnly:   m.config.Mode == MemoryCardPhysical,
		TitleCount: len(hashed), DiscCount: totalDiscs,
		CatalogFingerprint: fingerprint, Complete: true,
	}
	if err = m.createImage(ctx, imagePath, hashed, &manifest); err != nil {
		return LibraryManifest{}, err
	}
	checksums := make(map[string]string, len(manifest.Titles))
	for _, title := range manifest.Titles {
		checksums[title.OutputDir] = title.PreparedID
	}
	checksumData, err := json.MarshalIndent(map[string]any{
		"schema": LibrarySchema, "prepared_artifacts": checksums,
	}, "", "  ")
	if err != nil {
		return LibraryManifest{}, err
	}
	if err = os.WriteFile(filepath.Join(staging, "checksums.json"),
		append(checksumData, '\n'), 0o600); err != nil {
		return LibraryManifest{}, err
	}
	stagedManifest := manifest
	stagedManifest.ImagePath = imagePath
	if err = ValidateLibraryManifest(m.root, stagedManifest); err != nil {
		return LibraryManifest{}, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return LibraryManifest{}, err
	}
	data = append(data, '\n')
	if err = os.WriteFile(filepath.Join(staging, "manifest.json"), data, 0o600); err != nil {
		return LibraryManifest{}, err
	}
	if err = syncDirectory(staging); err != nil {
		return LibraryManifest{}, err
	}
	if err = os.Rename(staging, final); err != nil {
		return LibraryManifest{}, err
	}
	if err = syncDirectory(generations); err != nil {
		return LibraryManifest{}, err
	}
	if err = m.promote(manifest); err != nil {
		return LibraryManifest{}, err
	}
	return manifest, nil
}

func validateFSTFileSizes(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Size() > fat32MaximumFileSize {
				return fmt.Errorf("extracted FST file %s exceeds the FAT32 file limit",
					filepath.Base(path))
			}
		}
		return nil
	})
}

func CalculateLibrarySize(payloadBytes int64, config LibraryConfig) (int64, error) {
	if err := config.Validate(); err != nil {
		return 0, err
	}
	if payloadBytes < 0 {
		return 0, errors.New("negative GameCube payload size")
	}
	if payloadBytes > math.MaxInt64/100 {
		return 0, errors.New("GameCube library size overflow")
	}
	headroom := payloadBytes * int64(config.HeadroomPercent) / 100
	saveReserve := config.SaveReserveMiB << 20
	if payloadBytes > math.MaxInt64-headroom ||
		payloadBytes+headroom > math.MaxInt64-saveReserve-libraryMetadataReserve {
		return 0, errors.New("GameCube library size overflow")
	}
	size := payloadBytes + headroom + saveReserve + libraryMetadataReserve
	if size < libraryMinimumSize {
		size = libraryMinimumSize
	}
	if size > math.MaxInt64-(libraryAlignment-1) {
		return 0, errors.New("GameCube library size overflow")
	}
	size = (size + libraryAlignment - 1) / libraryAlignment * libraryAlignment
	// MBR LBA fields are 32-bit with 512-byte sectors.
	if size/VolumeSectorSize > math.MaxUint32 {
		return 0, errors.New("GameCube library exceeds MBR/FAT32 capacity")
	}
	if config.MaxVolumeGiB > 0 && size > config.MaxVolumeGiB<<30 {
		return 0, fmt.Errorf("GameCube library requires %d GiB; configured maximum is %d GiB",
			size>>30, config.MaxVolumeGiB)
	}
	return size, nil
}

func catalogFingerprint(games []Game) string {
	hash := sha256.New()
	for _, game := range games {
		fmt.Fprintf(hash, "%s\x00%d\x00%s", game.ID, game.Revision, game.Title)
		for _, disc := range game.Discs {
			fmt.Fprintf(hash, "\x00%d\x00%s\x00%d\x00%s",
				disc.Number, disc.Format, disc.PhysicalSize, disc.SHA256)
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (m *LibraryManager) createImage(ctx context.Context, path string, games []Game, manifest *LibraryManifest) error {
	disk, err := diskfs.Create(path, manifest.VolumeSize, diskfs.SectorSizeDefault)
	if err != nil {
		return err
	}
	defer disk.Close()
	sectors := uint32(manifest.VolumeSize / VolumeSectorSize)
	table := &mbr.Table{
		LogicalSectorSize: int(VolumeSectorSize), PhysicalSectorSize: int(VolumeSectorSize),
		Partitions: []*mbr.Partition{{
			Index: 1, Type: mbr.Fat32LBA, Start: VolumeStartSector,
			Size: sectors - VolumeStartSector,
		}},
	}
	if err = disk.Partition(table); err != nil {
		return err
	}
	fat, err := disk.CreateFilesystem(diskpkg.FilesystemSpec{
		Partition: 1, FSType: filesystem.TypeFat32, VolumeLabel: "WIIBRIDGE",
		Reproducible: true,
	})
	if err != nil {
		return err
	}
	defer fat.Close()
	for _, directory := range []string{"/games", "/saves", "/controllers", "/apps", "/apps/Nintendont"} {
		if err = fat.Mkdir(directory); err != nil {
			return err
		}
	}
	paths := make(map[string]struct{}, len(games))
	var bytesWritten int64
	discsCompleted := 0
	for gameIndex, game := range games {
		if err = ctx.Err(); err != nil {
			return err
		}
		outputDir, err := libraryOutputDir(game)
		if err != nil {
			return err
		}
		key := strings.ToLower(outputDir)
		if _, exists := paths[key]; exists {
			return fmt.Errorf("duplicate GameCube output path %q", outputDir)
		}
		paths[key] = struct{}{}
		if err = fat.Mkdir("/games/" + outputDir); err != nil {
			return err
		}
		preparedHash := sha256.New()
		format := ""
		for discIndex, disc := range game.Discs {
			if err = ctx.Err(); err != nil {
				return err
			}
			format = disc.Format
			fmt.Fprint(preparedHash, disc.SHA256)
			if disc.Format == "fst" {
				if len(game.Discs) != 1 {
					return errors.New("two-disc extracted FST sets are unsupported")
				}
				err = copyTreeToFilesystem(ctx, fat, disc.SourcePath, "/games/"+outputDir)
			} else {
				err = copyFileToFilesystem(ctx, fat, disc.SourcePath,
					"/games/"+outputDir+"/"+runtimeName(disc, discIndex == 1))
			}
			if err != nil {
				return err
			}
			bytesWritten += disc.PhysicalSize
			discsCompleted++
			m.setCurrent(game.Title, gameIndex, discsCompleted, bytesWritten)
		}
		manifest.Titles = append(manifest.Titles, LibraryTitle{
			Title: game.Title, ID: game.ID, Revision: game.Revision,
			Region: game.Region, Format: format, DiscCount: len(game.Discs),
			OutputDir:  "/games/" + outputDir,
			PreparedID: hex.EncodeToString(preparedHash.Sum(nil)),
		})
		m.setCurrent(game.Title, gameIndex+1, discsCompleted, bytesWritten)
	}
	if err = fat.Close(); err != nil {
		return err
	}
	manifest.ClusterSize, err = patchAndReadGeometry(path)
	return err
}

func libraryOutputDir(game Game) (string, error) {
	if game.ID == "" || strings.Contains(game.Title, "../") ||
		strings.Contains(game.Title, `..\`) {
		return "", errors.New("unsafe GameCube title path")
	}
	title := volumeTitle(game)
	if game.Revision > 0 {
		title += fmt.Sprintf(" [Rev %d]", game.Revision)
	}
	base := strings.TrimSpace(strings.SplitN(title, " ", 2)[0])
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4",
		"COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3",
		"LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		title = "_" + title
	}
	if !strings.Contains(title, "["+game.ID+"]") {
		return "", errors.New("GameCube output directory lacks game ID")
	}
	return title, nil
}

func patchAndReadGeometry(path string) (int, error) {
	if err := patchGeometry(path); err != nil {
		return 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	boot := make([]byte, 512)
	if _, err = file.ReadAt(boot, int64(VolumeStartSector)*VolumeSectorSize); err != nil {
		return 0, err
	}
	return int(binary.LittleEndian.Uint16(boot[11:13])) * int(boot[13]), nil
}

func (m *LibraryManager) promote(manifest LibraryManifest) error {
	pointer := struct {
		Schema       int    `json:"schema"`
		GenerationID string `json:"generation_id"`
	}{LibrarySchema, manifest.GenerationID}
	data, err := json.MarshalIndent(pointer, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(m.root, ".active-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err = temp.Chmod(0o600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tempName, filepath.Join(m.root, "active.json")); err != nil {
		return err
	}
	return syncDirectory(m.root)
}

func (m *LibraryManager) Active() (LibraryManifest, error) {
	data, err := os.ReadFile(filepath.Join(m.root, "active.json"))
	if err != nil {
		return LibraryManifest{}, err
	}
	var pointer struct {
		Schema       int    `json:"schema"`
		GenerationID string `json:"generation_id"`
	}
	if err = json.Unmarshal(data, &pointer); err != nil {
		return LibraryManifest{}, err
	}
	if pointer.Schema != LibrarySchema || !safeGenerationID(pointer.GenerationID) {
		return LibraryManifest{}, errors.New("invalid GameCube active-generation pointer")
	}
	manifestPath := filepath.Join(m.root, "generations", pointer.GenerationID, "manifest.json")
	return LoadAndValidateLibrary(m.root, manifestPath)
}

func safeGenerationID(value string) bool {
	return value != "" && filepath.Base(value) == value &&
		!strings.Contains(value, "..") && !strings.ContainsAny(value, `/\`)
}

func LoadAndValidateLibrary(root, manifestPath string) (LibraryManifest, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return LibraryManifest{}, err
	}
	manifestAbs, err := filepath.Abs(manifestPath)
	if err != nil {
		return LibraryManifest{}, err
	}
	if !strings.HasPrefix(manifestAbs, rootAbs+string(os.PathSeparator)) {
		return LibraryManifest{}, errors.New("GameCube library manifest escapes managed storage")
	}
	info, err := os.Lstat(manifestAbs)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return LibraryManifest{}, errors.New("invalid GameCube library manifest")
	}
	data, err := os.ReadFile(manifestAbs)
	if err != nil {
		return LibraryManifest{}, err
	}
	var manifest LibraryManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return LibraryManifest{}, err
	}
	if err = ValidateLibraryManifest(root, manifest); err != nil {
		return LibraryManifest{}, err
	}
	checksumPath := filepath.Join(filepath.Dir(manifestAbs), "checksums.json")
	checksumInfo, checksumErr := os.Lstat(checksumPath)
	if checksumErr != nil || !checksumInfo.Mode().IsRegular() ||
		checksumInfo.Mode()&os.ModeSymlink != 0 {
		return LibraryManifest{}, errors.New("missing GameCube prepared-artifact checksums")
	}
	checksumData, checksumErr := os.ReadFile(checksumPath)
	if checksumErr != nil {
		return LibraryManifest{}, checksumErr
	}
	var checksumDocument struct {
		Schema    int               `json:"schema"`
		Artifacts map[string]string `json:"prepared_artifacts"`
	}
	if checksumErr = json.Unmarshal(checksumData, &checksumDocument); checksumErr != nil ||
		checksumDocument.Schema != LibrarySchema ||
		len(checksumDocument.Artifacts) != len(manifest.Titles) {
		return LibraryManifest{}, errors.New("invalid GameCube prepared-artifact checksums")
	}
	for _, title := range manifest.Titles {
		if checksumDocument.Artifacts[title.OutputDir] != title.PreparedID {
			return LibraryManifest{}, errors.New("GameCube prepared-artifact checksum mismatch")
		}
	}
	return manifest, nil
}

func ValidateLibraryManifest(root string, manifest LibraryManifest) error {
	if manifest.Schema != LibrarySchema || !manifest.Complete ||
		!safeGenerationID(manifest.GenerationID) || manifest.Filesystem != "fat32" {
		return errors.New("stale or incomplete GameCube library manifest")
	}
	if manifest.TitleCount == 0 || manifest.TitleCount != len(manifest.Titles) {
		return errors.New("GameCube library manifest title count mismatch")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	imageAbs, err := filepath.Abs(manifest.ImagePath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(imageAbs, rootAbs+string(os.PathSeparator)) {
		return errors.New("GameCube library image escapes managed storage")
	}
	info, err := os.Lstat(imageAbs)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() != manifest.VolumeSize {
		return errors.New("GameCube library image size or type mismatch")
	}
	disk, err := diskfs.Open(imageAbs, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		return err
	}
	defer disk.Close()
	fat, err := disk.GetFilesystem(1)
	if err != nil {
		return err
	}
	defer fat.Close()
	if _, err = fat.Stat("games"); err != nil {
		return fmt.Errorf("GameCube /games directory: %w", err)
	}
	seen := make(map[string]struct{}, len(manifest.Titles))
	discCount := 0
	for _, title := range manifest.Titles {
		if title.ID == "" || title.DiscCount < 1 || title.DiscCount > 2 ||
			!strings.HasPrefix(title.OutputDir, "/games/") {
			return errors.New("invalid GameCube title manifest entry")
		}
		key := strings.ToLower(title.OutputDir)
		if _, exists := seen[key]; exists {
			return errors.New("duplicate GameCube title output path")
		}
		seen[key] = struct{}{}
		base := strings.TrimPrefix(title.OutputDir, "/")
		required := "game.iso"
		if title.Format == "ciso" {
			required = "game.ciso"
		}
		if title.Format == "fst" {
			required = "sys/boot.bin"
		}
		if _, err = fat.Stat(base + "/" + required); err != nil {
			return fmt.Errorf("missing %s for %s", required, title.ID)
		}
		if title.DiscCount == 2 {
			second := "disc2.iso"
			if title.Format == "ciso" {
				second = "disc2.ciso"
			}
			if _, err = fat.Stat(base + "/" + second); err != nil {
				return fmt.Errorf("missing %s for %s", second, title.ID)
			}
		}
		discCount += title.DiscCount
	}
	if discCount != manifest.DiscCount {
		return errors.New("GameCube library manifest disc count mismatch")
	}
	return nil
}

func (m *LibraryManager) Current(games []Game) (bool, error) {
	active, err := m.Active()
	if err != nil {
		return false, err
	}
	hashed := make([]Game, len(games))
	for index, game := range games {
		hashed[index], err = hashGameSources(game)
		if err != nil {
			return false, err
		}
	}
	sort.Slice(hashed, func(i, j int) bool {
		if hashed[i].ID == hashed[j].ID {
			return hashed[i].Revision < hashed[j].Revision
		}
		return hashed[i].ID < hashed[j].ID
	})
	return active.CatalogFingerprint == catalogFingerprint(hashed), nil
}

func (m *LibraryManager) prune(active string) error {
	entries, err := os.ReadDir(filepath.Join(m.root, "generations"))
	if err != nil {
		return err
	}
	var complete []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".building-") {
			complete = append(complete, entry)
		}
	}
	sort.Slice(complete, func(i, j int) bool { return complete[i].Name() > complete[j].Name() })
	kept := 1
	for _, entry := range complete {
		if entry.Name() == active {
			continue
		}
		if kept < m.config.Retention {
			kept++
			continue
		}
		_ = os.RemoveAll(filepath.Join(m.root, "generations", entry.Name()))
	}
	return nil
}

// BackupLibraryMemoryCards snapshots validated Nintendont card files only
// after the aggregate image is detached from USB and NBD.
func BackupLibraryMemoryCards(manifest LibraryManifest, backupRoot string, retain int) error {
	if manifest.Mode != MemoryCardEmulated {
		return nil
	}
	if retain < DefaultSaveBackupRetention {
		return fmt.Errorf("save retention must be at least %d", DefaultSaveBackupRetention)
	}
	managedRoot := filepath.Dir(filepath.Dir(filepath.Dir(manifest.ImagePath)))
	if err := ValidateLibraryManifest(managedRoot, manifest); err != nil {
		return fmt.Errorf("pre-backup GameCube library validation: %w", err)
	}
	disk, err := diskfs.Open(manifest.ImagePath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		return err
	}
	defer disk.Close()
	fat, err := disk.GetFilesystem(1)
	if err != nil {
		return err
	}
	defer fat.Close()
	entries, err := fat.ReadDir("saves")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".raw") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !validMemoryCardSizes[info.Size()] {
			return fmt.Errorf("refusing invalid memory card %s (%d bytes)", entry.Name(), info.Size())
		}
		source, openErr := fat.Open("saves/" + entry.Name())
		if openErr != nil {
			return openErr
		}
		directory := filepath.Join(backupRoot, "library", entry.Name())
		if err = os.MkdirAll(directory, 0o700); err != nil {
			source.Close()
			return err
		}
		final := filepath.Join(directory,
			time.Now().UTC().Format("20060102T150405.000000000Z")+".raw")
		temp, createErr := os.CreateTemp(directory, ".save-backup-")
		if createErr != nil {
			source.Close()
			return createErr
		}
		tempName := temp.Name()
		copyErr := temp.Chmod(0o600)
		if copyErr == nil {
			_, copyErr = io.CopyN(temp, source, info.Size())
		}
		if copyErr == nil {
			copyErr = temp.Sync()
		}
		if closeErr := temp.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if closeErr := source.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr == nil {
			copyErr = os.Rename(tempName, final)
		}
		_ = os.Remove(tempName)
		if copyErr != nil {
			return copyErr
		}
		if err = rotateSaveBackups(directory, retain); err != nil {
			return err
		}
	}
	return nil
}
