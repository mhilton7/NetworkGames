package gamecube

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/filesystem"
)

const DefaultSaveBackupRetention = 5

var validMemoryCardSizes = map[int64]bool{
	512 << 10: true,
	1 << 20:   true,
	2 << 20:   true,
	4 << 20:   true,
	8 << 20:   true,
	16 << 20:  true,
}

type SaveBackup struct {
	GameID   string    `json:"game_id"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Created  time.Time `json:"created_utc"`
	Revision byte      `json:"revision"`
}

func ListSaveBackups(root, gameID string, revision byte) ([]SaveBackup, error) {
	base := filepath.Join(root, gameID, fmt.Sprintf("r%d", revision))
	var backups []SaveBackup
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".raw") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !validMemoryCardSizes[info.Size()] {
			return nil
		}
		backups = append(backups, SaveBackup{
			GameID: gameID, Revision: revision, Name: filepath.Base(filepath.Dir(path)),
			Path: path, Size: info.Size(), Created: info.ModTime().UTC(),
		})
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Created.After(backups[j].Created) })
	return backups, err
}

// BackupMemoryCards must only be called after the USB gadget and all NBD
// sessions have been detached. It never mounts the exported volume.
func BackupMemoryCards(manifest VolumeManifest, backupRoot string, retain int) ([]SaveBackup, error) {
	if manifest.Mode != MemoryCardEmulated {
		return nil, nil
	}
	if retain < DefaultSaveBackupRetention {
		return nil, fmt.Errorf("save retention must be at least %d", DefaultSaveBackupRetention)
	}
	if _, err := ValidateVolume(manifest.ImagePath, manifest); err != nil {
		return nil, fmt.Errorf("pre-backup FAT validation: %w", err)
	}
	disk, err := diskfs.Open(manifest.ImagePath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		return nil, err
	}
	defer disk.Close()
	fat, err := disk.GetFilesystem(1)
	if err != nil {
		return nil, err
	}
	defer fat.Close()
	entries, err := fat.ReadDir("saves")
	if err != nil {
		return nil, err
	}
	var backups []SaveBackup
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".raw") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		if !validMemoryCardSizes[info.Size()] {
			return nil, fmt.Errorf("refusing invalid memory card %s (%d bytes)", entry.Name(), info.Size())
		}
		source, openErr := fat.Open("saves/" + entry.Name())
		if openErr != nil {
			return nil, openErr
		}
		backup, copyErr := writeSaveBackup(source, info.Size(), backupRoot, manifest, entry.Name())
		closeErr := source.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		backups = append(backups, backup)
		if err = rotateSaveBackups(filepath.Dir(backup.Path), retain); err != nil {
			return nil, err
		}
	}
	return backups, nil
}

func writeSaveBackup(source fs.File, size int64, root string, manifest VolumeManifest, name string) (SaveBackup, error) {
	directory := filepath.Join(root, manifest.Game.ID, fmt.Sprintf("r%d", manifest.Game.Revision), name)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return SaveBackup{}, err
	}
	now := time.Now().UTC()
	final := filepath.Join(directory, now.Format("20060102T150405.000000000Z")+".raw")
	temp, err := os.CreateTemp(directory, ".save-backup-")
	if err != nil {
		return SaveBackup{}, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err = temp.Chmod(0o600); err == nil {
		var written int64
		written, err = io.CopyN(temp, source, size)
		if err == nil && written != size {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return SaveBackup{}, err
	}
	if err = os.Rename(tempPath, final); err != nil {
		return SaveBackup{}, err
	}
	if err = syncDirectory(directory); err != nil {
		return SaveBackup{}, err
	}
	return SaveBackup{
		GameID: manifest.Game.ID, Name: name, Path: final, Size: size,
		Created: now, Revision: manifest.Game.Revision,
	}, nil
}

func rotateSaveBackups(directory string, retain int) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".raw") {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(paths)
	for len(paths) > retain {
		if err = os.Remove(paths[0]); err != nil {
			return err
		}
		paths = paths[1:]
	}
	return syncDirectory(directory)
}

// RestoreMemoryCard writes one validated backup into a detached GameCube
// volume. The caller must first back up the current cards.
func RestoreMemoryCard(manifest VolumeManifest, backupPath, saveName string) error {
	if manifest.Mode != MemoryCardEmulated {
		return errors.New("physical memory-card mode has no USB save to restore")
	}
	cleanName := filepath.Base(saveName)
	if cleanName != saveName || !strings.EqualFold(filepath.Ext(cleanName), ".raw") {
		return errors.New("invalid memory-card filename")
	}
	source, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !validMemoryCardSizes[info.Size()] {
		return fmt.Errorf("invalid backup memory-card size %d", info.Size())
	}
	if _, err = ValidateVolume(manifest.ImagePath, manifest); err != nil {
		return fmt.Errorf("pre-restore FAT validation: %w", err)
	}
	disk, err := diskfs.Open(manifest.ImagePath, diskfs.WithOpenMode(diskfs.ReadWrite))
	if err != nil {
		return err
	}
	defer disk.Close()
	fat, err := disk.GetFilesystem(1)
	if err != nil {
		return err
	}
	defer fat.Close()
	target, err := fat.OpenFile("saves/"+cleanName, os.O_CREATE|os.O_RDWR|os.O_TRUNC)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != info.Size() {
		return io.ErrShortWrite
	}
	if _, err = ValidateVolume(manifest.ImagePath, manifest); err != nil {
		return fmt.Errorf("post-restore FAT validation: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func readSave(fat filesystem.FileSystem, name string) ([]byte, error) {
	file, err := fat.Open("saves/" + name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}
