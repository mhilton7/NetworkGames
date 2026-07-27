package gamecube

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"

	"wiibridge/tests/testutil"
)

func libraryGames(t *testing.T, root string) []Game {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name, id, title string
		disc, revision  byte
	}{
		{"alpha.iso", "GALP01", "Alpha/Game", 0, 0},
		{"beta-1.iso", "GBET01", "Beta Game", 0, 1},
		{"beta-2.iso", "GBET01", "Beta Game", 1, 1},
	}
	for _, fixture := range fixtures {
		path := filepath.Join(root, fixture.name)
		if err := testutil.SyntheticGameCubeISO(path, fixture.id, fixture.title,
			fixture.disc, fixture.revision, 2<<20); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 2 {
		t.Fatalf("games=%d rejections=%#v", len(result.Games), result.Rejected)
	}
	return result.Games
}

func TestCompleteLibraryBuildContainsEveryTitleAndDisc(t *testing.T) {
	root := t.TempDir()
	games := libraryGames(t, filepath.Join(root, "sources"))
	before := make(map[string]string)
	for _, game := range games {
		for _, disc := range game.Discs {
			sum, err := hashFile(disc.SourcePath)
			if err != nil {
				t.Fatal(err)
			}
			before[disc.SourcePath] = sum
		}
	}
	manager, err := NewLibraryManager(filepath.Join(root, "managed"), DefaultLibraryConfig())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := manager.Build(context.Background(), games)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.TitleCount != 2 || manifest.DiscCount != 3 || !manifest.ReadOnly {
		t.Fatalf("manifest=%#v", manifest)
	}
	active, err := manager.Active()
	if err != nil {
		t.Fatal(err)
	}
	if active.GenerationID != manifest.GenerationID {
		t.Fatalf("active generation=%q want %q", active.GenerationID, manifest.GenerationID)
	}
	disk, err := diskfs.Open(manifest.ImagePath, diskfs.WithOpenMode(diskfs.ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	fat, err := disk.GetFilesystem(1)
	if err != nil {
		t.Fatal(err)
	}
	defer fat.Close()
	for _, title := range manifest.Titles {
		if !strings.Contains(title.OutputDir, "["+title.ID+"]") {
			t.Fatalf("output path lacks ID: %q", title.OutputDir)
		}
		if _, err = fat.Stat(strings.TrimPrefix(title.OutputDir, "/") + "/game.iso"); err != nil {
			t.Fatal(err)
		}
		if title.DiscCount == 2 {
			if _, err = fat.Stat(strings.TrimPrefix(title.OutputDir, "/") + "/disc2.iso"); err != nil {
				t.Fatal(err)
			}
		}
	}
	for path, expected := range before {
		actual, hashErr := hashFile(path)
		if hashErr != nil || actual != expected {
			t.Fatalf("source changed: path=%s hash=%s err=%v", path, actual, hashErr)
		}
	}
}

func TestLibrarySizingLimitsAndOverflow(t *testing.T) {
	config := DefaultLibraryConfig()
	size, err := CalculateLibrarySize(4<<30, config)
	if err != nil {
		t.Fatal(err)
	}
	if size < 5<<30 || size%libraryAlignment != 0 {
		t.Fatalf("unexpected volume size %d", size)
	}
	config.MaxVolumeGiB = 8
	if _, err = CalculateLibrarySize(20<<30, config); err == nil {
		t.Fatal("configured maximum was ignored")
	}
	if _, err = CalculateLibrarySize(math.MaxInt64, DefaultLibraryConfig()); err == nil {
		t.Fatal("integer overflow was accepted")
	}
}

func TestLibraryRejectsUnsafeTitleAndFAT32Overflow(t *testing.T) {
	if _, err := libraryOutputDir(Game{ID: "GTEST1", Title: "../escape"}); err == nil {
		t.Fatal("path traversal title was accepted")
	}
	manager, err := NewLibraryManager(filepath.Join(t.TempDir(), "managed"), DefaultLibraryConfig())
	if err != nil {
		t.Fatal(err)
	}
	game := Game{
		ID: "GTEST1", Title: "Too Large", Validation: "valid",
		Discs: []Disc{{
			ID: "GTEST1", Number: 0, Format: "iso", Validation: "valid",
			SourcePath: "/not/read", PhysicalSize: fat32MaximumFileSize + 1,
		}},
	}
	if _, err = manager.Build(context.Background(), []Game{game}); err == nil {
		t.Fatal("FAT32 oversized source was accepted")
	}
}

func TestInvalidGenerationCannotReplaceActive(t *testing.T) {
	root := t.TempDir()
	games := libraryGames(t, filepath.Join(root, "sources"))
	manager, err := NewLibraryManager(filepath.Join(root, "managed"), DefaultLibraryConfig())
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Build(context.Background(), games)
	if err != nil {
		t.Fatal(err)
	}
	activeBefore, err := os.ReadFile(filepath.Join(manager.Root(), "active.json"))
	if err != nil {
		t.Fatal(err)
	}
	bad := games[0]
	bad.Discs[0].SourcePath = filepath.Join(root, "missing.iso")
	if _, err = manager.Build(context.Background(), []Game{bad}); err == nil {
		t.Fatal("invalid rebuild succeeded")
	}
	activeAfter, err := os.ReadFile(filepath.Join(manager.Root(), "active.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(activeAfter) != string(activeBefore) {
		t.Fatal("failed build replaced active generation")
	}
	active, err := manager.Active()
	if err != nil || active.GenerationID != first.GenerationID {
		t.Fatalf("active generation lost: %#v err=%v", active, err)
	}
}

func TestManifestEscapeMissingImageAndSizeMismatchRejected(t *testing.T) {
	root := t.TempDir()
	manifest := LibraryManifest{
		Schema: LibrarySchema, GenerationID: "safe", Complete: true,
		Filesystem: "fat32", ImagePath: filepath.Join(root, "..", "escape.img"),
		TitleCount: 1, Titles: []LibraryTitle{{ID: "GTEST1", DiscCount: 1}},
	}
	if err := ValidateLibraryManifest(root, manifest); err == nil {
		t.Fatal("manifest path escape accepted")
	}
	manifest.ImagePath = filepath.Join(root, "missing.img")
	if err := ValidateLibraryManifest(root, manifest); err == nil {
		t.Fatal("missing image accepted")
	}
	if err := os.WriteFile(manifest.ImagePath, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest.VolumeSize = 99
	if err := ValidateLibraryManifest(root, manifest); err == nil {
		t.Fatal("image size mismatch accepted")
	}
}

func TestActivePointerRejectsTraversal(t *testing.T) {
	manager, err := NewLibraryManager(filepath.Join(t.TempDir(), "managed"), DefaultLibraryConfig())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"schema": LibrarySchema, "generation_id": "../escape"})
	if err = os.WriteFile(filepath.Join(manager.Root(), "active.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Active(); err == nil {
		t.Fatal("active generation traversal accepted")
	}
}
