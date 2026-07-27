package gamecube

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wiibridge/tests/testutil"
)

func TestScanISOAndGCMMetadataRegionsAndUnicodePaths(t *testing.T) {
	root := t.TempDir()
	if err := testutil.SyntheticGameCubeISO(
		filepath.Join(root, "Pokémon テスト.iso"), "GTEE01", "Synthetic USA", 0, 2, 2<<20); err != nil {
		t.Fatal(err)
	}
	if err := testutil.SyntheticGameCubeISO(
		filepath.Join(root, "PAL.gcm"), "GTPP01", "Synthetic PAL", 0, 1, 2<<20); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 2 || len(result.Rejected) != 0 {
		t.Fatalf("unexpected scan result: %#v", result)
	}
	if result.Games[0].ID != "GTEE01" || result.Games[0].Region != "NTSC-U" ||
		result.Games[0].Revision != 2 || result.Games[0].Format != "iso" ||
		len(result.Games[0].Discs[0].SHA256) != 64 {
		t.Fatalf("unexpected first game metadata: %#v", result.Games[0])
	}
	if result.Games[1].Region != "PAL" || result.Games[1].Format != "gcm" {
		t.Fatalf("unexpected second game metadata: %#v", result.Games[1])
	}
}

func TestScanPairsTwoDiscsByHeaderMetadata(t *testing.T) {
	root := t.TempDir()
	for disc, name := range []string{"not-disc-one-name.iso", "unrelated-filename.gcm"} {
		if err := testutil.SyntheticGameCubeISO(
			filepath.Join(root, name), "GTDE01", "Synthetic Pair", byte(disc), 3, 2<<20); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 1 || result.Games[0].DiscCount != 2 ||
		result.Games[0].Discs[0].Number != 0 || result.Games[0].Discs[1].Number != 1 {
		t.Fatalf("two-disc set was not paired by metadata: %#v", result)
	}
}

func TestScanRejectsDuplicateDiscAndOrphanDiscTwo(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.iso", "b.gcm"} {
		if err := testutil.SyntheticGameCubeISO(
			filepath.Join(root, name), "GDUP01", "Duplicate", 0, 0, 2<<20); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.SyntheticGameCubeISO(
		filepath.Join(root, "orphan.iso"), "GORP01", "Orphan", 1, 0, 2<<20); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 0 || len(result.Rejected) < 2 {
		t.Fatalf("duplicates or orphan were accepted: %#v", result)
	}
	reasons := result.Rejected[0].Reason + " " + result.Rejected[1].Reason
	if !strings.Contains(reasons, "duplicate") && !strings.Contains(reasons, "disc two") {
		t.Fatalf("unexpected rejection reasons: %#v", result.Rejected)
	}
}

func TestScanAllowsMultipleRevisions(t *testing.T) {
	root := t.TempDir()
	for revision := byte(0); revision < 2; revision++ {
		if err := testutil.SyntheticGameCubeISO(
			filepath.Join(root, "revision-"+string(rune('0'+revision))+".iso"),
			"GREV01", "Revision", 0, revision, 2<<20); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 2 || result.Games[0].Revision == result.Games[1].Revision {
		t.Fatalf("revisions were incorrectly collapsed: %#v", result)
	}
}

func TestScanSupportsNintendontCISOWithTwoMiBBlocks(t *testing.T) {
	root := t.TempDir()
	iso := filepath.Join(root, "source.tmp")
	if err := testutil.SyntheticGameCubeISO(iso, "GCSE01", "CISO", 0, 0, 2<<20); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(iso)
	if err != nil {
		t.Fatal(err)
	}
	ciso := make([]byte, int(cisoHeader)+len(data))
	copy(ciso[:4], "CISO")
	binary.LittleEndian.PutUint32(ciso[4:8], uint32(cisoBlock))
	ciso[8] = 1
	copy(ciso[cisoHeader:], data)
	if err = os.WriteFile(filepath.Join(root, "game.cso"), ciso, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(iso); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 1 || result.Games[0].Format != "ciso" {
		t.Fatalf("valid CISO was not accepted: %#v", result)
	}
}

func TestScanExtractedFST(t *testing.T) {
	root := t.TempDir()
	gameRoot := filepath.Join(root, "Extracted [GFST01]")
	if err := writeSyntheticFST(gameRoot, "GFST01"); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 1 || result.Games[0].Format != "fst" ||
		result.Games[0].Discs[0].PhysicalSize == 0 {
		t.Fatalf("valid extracted FST was not accepted: %#v", result)
	}
}

func TestScanRejectsInvalidTruncatedUnsupportedAndSymlinkSources(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "truncated.iso"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := testutil.SyntheticGameCubeISO(
		filepath.Join(root, "invalid.nkit.iso"), "GNKT01", "NKit", 0, 0, 2<<20); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "bad.gcm")
	if err := testutil.SyntheticGameCubeISO(bad, "GBAD01", "Bad", 0, 0, 2<<20); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(bad, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteAt(make([]byte, 4), 0x1c); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err = os.Symlink(bad, filepath.Join(root, "linked.iso")); err != nil {
		t.Fatal(err)
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Games) != 0 || len(result.Rejected) != 4 {
		t.Fatalf("unsafe sources were not all rejected: %#v", result)
	}
}

func writeSyntheticFST(root, id string) error {
	if err := os.MkdirAll(filepath.Join(root, "sys"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "files"), 0o700); err != nil {
		return err
	}
	temp := filepath.Join(root, "disc.tmp")
	if err := testutil.SyntheticGameCubeISO(temp, id, "Extracted", 0, 0, 2<<20); err != nil {
		return err
	}
	data, err := os.ReadFile(temp)
	if err != nil {
		return err
	}
	if err = os.Remove(temp); err != nil {
		return err
	}
	files := map[string][]byte{
		"sys/boot.bin":      data[:0x440],
		"sys/bi2.bin":       make([]byte, 0x20),
		"sys/apploader.img": {1},
		"sys/main.dol":      {1},
		"files/fixture.bin": {1, 2, 3},
	}
	for relative, contents := range files {
		if err = os.WriteFile(filepath.Join(root, filepath.FromSlash(relative)), contents, 0o600); err != nil {
			return err
		}
	}
	return nil
}
