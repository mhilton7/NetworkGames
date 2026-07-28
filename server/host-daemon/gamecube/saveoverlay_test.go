package gamecube

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"wiibridge/shared/perf"
)

func testSaveObject(t *testing.T, root string) SaveObject {
	t.Helper()
	objects, err := PlanSaveObjects(MemoryCardEmulatedIndividual,
		[]Game{{ID: "GSAV01"}}, "", 512<<10)
	if err != nil {
		t.Fatal(err)
	}
	if err = EnsureSaveObjects(root, objects, true, "test", "generation-1"); err != nil {
		t.Fatal(err)
	}
	return objects[0]
}

func openTestSaveStore(t *testing.T, root string, object SaveObject) *SaveStore {
	t.Helper()
	store, err := OpenSaveStore(SaveStoreConfig{
		Root: root, Application: "test", GenerationID: "generation-1",
		LayoutChecksum: "layout", MaxBackups: 3,
		Metrics: perf.New(perf.Config{Enabled: true}),
	}, []SaveObject{object})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestPlanIndividualAndSharedCards(t *testing.T) {
	games := []Game{{ID: "GSAV01"}, {ID: "GSAV02"}}
	individual, err := PlanSaveObjects(
		MemoryCardEmulatedIndividual, games, "", 512<<10)
	if err != nil || len(individual) != 2 ||
		individual[0].VirtualPath != "/saves/GSAV01.raw" {
		t.Fatalf("individual=%#v err=%v", individual, err)
	}
	shared, err := PlanSaveObjects(
		MemoryCardEmulatedShared, games, "family", 1<<20)
	if err != nil || len(shared) != 1 ||
		shared[0].VirtualPath != "/saves/ninmem.raw" {
		t.Fatalf("shared=%#v err=%v", shared, err)
	}
	if _, err = PlanSaveObjects(
		MemoryCardEmulatedShared, games, "../escape", 1<<20); err == nil {
		t.Fatal("unsafe shared name accepted")
	}
	if _, err = PlanSaveObjects(
		MemoryCardEmulatedIndividual, games, "", 12345); err == nil {
		t.Fatal("unsupported card size accepted")
	}
	for _, size := range []int64{
		512 << 10, 1 << 20, 2 << 20, 4 << 20, 8 << 20, 16 << 20,
	} {
		if !SupportedSaveCardSize(size) {
			t.Fatalf("supported Nintendont card size %d was rejected", size)
		}
	}
}

func TestSharedSaveCardCreationAndPersistence(t *testing.T) {
	root := t.TempDir()
	objects, err := PlanSaveObjects(
		MemoryCardEmulatedShared, []Game{{ID: "GSAV01"}, {ID: "GSAV02"}},
		"family", 1<<20)
	if err != nil || len(objects) != 1 {
		t.Fatalf("objects=%#v err=%v", objects, err)
	}
	if err = EnsureSaveObjects(root, objects, true, "test", "generation-1"); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSaveStore(SaveStoreConfig{
		Root: root, Application: "test", GenerationID: "generation-1",
		LayoutChecksum: "layout", MaxBackups: 3,
	}, objects)
	if err != nil {
		t.Fatal(err)
	}
	expected := bytes.Repeat([]byte{0x71}, 1024)
	if _, err = store.WriteSaveAt("shared:family", expected, 4096); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenSaveStore(SaveStoreConfig{
		Root: root, Application: "test", GenerationID: "generation-2",
		LayoutChecksum: "layout-2", MaxBackups: 3,
	}, objects)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	actual := make([]byte, len(expected))
	if _, err = store.ReadSaveAt("shared:family", actual, 4096); err != nil ||
		!bytes.Equal(actual, expected) {
		t.Fatalf("shared card mismatch: %v", err)
	}
}

func TestSaveStoreWriteFlushAndReopen(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store := openTestSaveStore(t, root, object)
	expected := bytes.Repeat([]byte{0x5a}, 2048)
	if count, err := store.WriteSaveAt(object.ID, expected, 1024); err != nil ||
		count != len(expected) {
		t.Fatalf("write=%d err=%v", count, err)
	}
	actual := make([]byte, len(expected))
	if _, err := store.ReadSaveAt(object.ID, actual, 1024); err != nil ||
		!bytes.Equal(actual, expected) {
		t.Fatalf("dirty read mismatch: %v", err)
	}
	if err := store.Sync(); err != nil {
		t.Fatal(err)
	}
	statuses := store.Statuses()
	if len(statuses) != 1 || statuses[0].Dirty || statuses[0].CurrentSHA256 == "" ||
		statuses[0].LastFlush.IsZero() {
		t.Fatalf("status=%#v", statuses)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestSaveStore(t, root, object)
	defer reopened.Close()
	clear(actual)
	if _, err := reopened.ReadSaveAt(object.ID, actual, 1024); err != nil ||
		!bytes.Equal(actual, expected) {
		t.Fatalf("reopened read mismatch: %v", err)
	}
}

func TestSaveStoreBoundsAndJournalLimit(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store, err := OpenSaveStore(SaveStoreConfig{
		Root: root, Application: "test", GenerationID: "generation-1",
		LayoutChecksum: "layout", MaxDirtyBlocks: 1, MaxPendingBytes: 512,
		MaxJournalBytes: 1024,
	}, []SaveObject{object})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.WriteSaveAt(object.ID, make([]byte, 1024), object.CardSize-512); err == nil {
		t.Fatal("out-of-card write accepted")
	}
	if _, err = store.WriteSaveAt("individual:OTHER1", []byte{1}, 0); err == nil {
		t.Fatal("unknown save object accepted")
	}
	if _, err = store.WriteSaveAt(object.ID, bytes.Repeat([]byte{1}, 512), 0); err != nil {
		t.Fatal(err)
	}
	// The next block forces a bounded checkpoint rather than unbounded growth.
	if _, err = store.WriteSaveAt(object.ID, bytes.Repeat([]byte{2}, 512), 512); err != nil {
		t.Fatal(err)
	}
	if status := store.Statuses()[0]; status.DirtyBlocks > 1 ||
		status.JournalBytes > 1024 {
		t.Fatalf("bounds exceeded: %#v", status)
	}
}

func TestInterruptedJournalRecoversDeterministically(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store := openTestSaveStore(t, root, object)
	expected := bytes.Repeat([]byte{0x33}, 512)
	if _, err := store.WriteSaveAt(object.ID, expected, 4096); err != nil {
		t.Fatal(err)
	}
	// Simulate a process crash without invoking Close, which intentionally
	// checkpoints pending data.
	card := store.objects[object.ID]
	if err := card.journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := card.file.Close(); err != nil {
		t.Fatal(err)
	}
	store.closed = true

	recovered := openTestSaveStore(t, root, object)
	defer recovered.Close()
	actual := make([]byte, len(expected))
	if _, err := recovered.ReadSaveAt(object.ID, actual, 4096); err != nil ||
		!bytes.Equal(actual, expected) {
		t.Fatalf("recovery mismatch: %v", err)
	}
	status := recovered.Statuses()[0]
	if status.Dirty || status.RecoveryState != "clean" {
		t.Fatalf("recovery status=%#v", status)
	}
}

func TestBackupRestoreUploadAndPruning(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store := openTestSaveStore(t, root, object)
	defer store.Close()

	first := bytes.Repeat([]byte{0x41}, int(object.CardSize))
	if err := store.Upload(object.ID, bytes.NewReader(first), object.CardSize); err != nil {
		t.Fatal(err)
	}
	backup, err := store.Backup(object.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 4; index++ {
		if _, err = store.Backup(object.ID, "automatic"); err != nil {
			t.Fatal(err)
		}
	}
	backups, err := store.ListBackups(object.ID)
	if err != nil || len(backups) != 3 {
		t.Fatalf("backups=%d err=%v", len(backups), err)
	}

	second := bytes.Repeat([]byte{0x42}, int(object.CardSize))
	if err = store.Upload(object.ID, bytes.NewReader(second), object.CardSize); err != nil {
		t.Fatal(err)
	}
	// The named original backup may have been pruned; restore a retained one
	// whose checksum matches the first uploaded card.
	restoreName := ""
	for _, item := range backups {
		if item.SHA256 == backup.SHA256 {
			restoreName = item.Name
			break
		}
	}
	if restoreName == "" {
		restoreName = backups[0].Name
	}
	if err = store.Restore(object.ID, restoreName); err != nil {
		t.Fatal(err)
	}
	file, _, err := store.OpenDownload(object.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(file.Name())
	file.Close()
	if err != nil || len(actual) != int(object.CardSize) {
		t.Fatalf("download size=%d err=%v", len(actual), err)
	}
}

func TestInvalidAndAmbiguousCardStateBlocksOpen(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	directory, _ := saveObjectDirectory(root, object)
	if err := os.WriteFile(filepath.Join(directory, ".checkpoint.tmp"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".restore.tmp"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSaveStore(SaveStoreConfig{
		Root: root, Application: "test", GenerationID: "generation-1",
		LayoutChecksum: "layout",
	}, []SaveObject{object}); err == nil ||
		(!errors.Is(errors.Unwrap(err), os.ErrNotExist) &&
			!strings.Contains(err.Error(), "SAVE-RECOVERY-AMBIGUOUS")) {
		t.Fatalf("ambiguous recovery was not blocked: %v", err)
	}
}

func TestRestoreStagesSelectedBackupBeforeRetentionPruning(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store, err := OpenSaveStore(SaveStoreConfig{
		Root: root, Application: "test", GenerationID: "generation-1",
		LayoutChecksum: "layout", MaxBackups: 1,
	}, []SaveObject{object})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first := bytes.Repeat([]byte{0x51}, int(object.CardSize))
	if err = store.Upload(object.ID, bytes.NewReader(first), object.CardSize); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Backup(object.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	second := bytes.Repeat([]byte{0x52}, int(object.CardSize))
	if err = store.Upload(object.ID, bytes.NewReader(second), object.CardSize); err != nil {
		t.Fatal(err)
	}
	backups, err := store.ListBackups(object.ID)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%#v err=%v", backups, err)
	}
	if err = store.Restore(object.ID, backups[0].Name); err != nil {
		t.Fatal(err)
	}
	file, _, err := store.OpenDownload(object.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	actual, readErr := os.ReadFile(file.Name())
	file.Close()
	if readErr != nil || !bytes.Equal(actual, first) {
		t.Fatalf("retention restore mismatch: %v", readErr)
	}
}

func TestOversizedSingleWriteFailsWithoutDiscardingDirtyData(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store, err := OpenSaveStore(SaveStoreConfig{
		Root: root, Application: "test", GenerationID: "generation-1",
		LayoutChecksum: "layout", MaxPendingBytes: 512,
		MaxJournalBytes: 1024, MaxDirtyBlocks: 1,
	}, []SaveObject{object})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.WriteSaveAt(object.ID, make([]byte, 513), 0); err == nil ||
		!strings.Contains(err.Error(), "SAVE-JOURNAL-LIMIT") {
		t.Fatalf("oversized write error=%v", err)
	}
	if status := store.Statuses()[0]; status.Dirty || status.JournalBytes != 0 {
		t.Fatalf("failed request mutated save state: %#v", status)
	}
}

func TestFlushFailureLeavesConfirmedCardUnchanged(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store := openTestSaveStore(t, root, object)
	card := store.objects[object.ID]
	before, err := hashRegularFile(card.activePath, object.CardSize)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.WriteSaveAt(object.ID, []byte("pending"), 0); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(card.directory, ".checkpoint.tmp")
	if err = os.Mkdir(blocker, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(blocker, "hold"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = store.Sync(); err == nil {
		t.Fatal("fault-injected flush succeeded")
	}
	after, err := hashRegularFile(card.activePath, object.CardSize)
	if err != nil || before != after {
		t.Fatalf("confirmed card changed: before=%s after=%s err=%v", before, after, err)
	}
	if status := store.Statuses()[0]; !status.Dirty {
		t.Fatalf("dirty data was silently discarded: %#v", status)
	}
	if err = os.Remove(filepath.Join(blocker, "hold")); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedActivationRollsBackToConfirmedCard(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store := openTestSaveStore(t, root, object)
	card := store.objects[object.ID]
	before, err := os.ReadFile(card.activePath)
	if err != nil {
		t.Fatal(err)
	}
	previousMetadata := card.metadata
	if err = preparePrevious(card); err != nil {
		t.Fatal(err)
	}
	if err = card.file.Close(); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(card.directory, ".synthetic-replacement")
	if err = os.WriteFile(replacement,
		bytes.Repeat([]byte{0x77}, int(object.CardSize)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(replacement, card.activePath); err != nil {
		t.Fatal(err)
	}
	card.metadata.SHA256 = strings.Repeat("7", 64)
	if err = writeSaveMetadata(card.metadataPath, card.metadata); err != nil {
		t.Fatal(err)
	}
	if err = card.journal.Close(); err != nil {
		t.Fatal(err)
	}
	store.closed = true

	recovered := openTestSaveStore(t, root, object)
	defer recovered.Close()
	file, _, err := recovered.OpenDownload(object.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	actual, readErr := os.ReadFile(file.Name())
	file.Close()
	if readErr != nil || !bytes.Equal(actual, before) {
		t.Fatalf("rollback mismatch: %v", readErr)
	}
	status := recovered.Statuses()[0]
	if status.CurrentSHA256 != previousMetadata.SHA256 ||
		status.RecoveryState != "rolled-back-interrupted-checkpoint" {
		t.Fatalf("recovery status=%#v", status)
	}
}

func TestBackupSymlinkRejectedForRestoreAndDownload(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store := openTestSaveStore(t, root, object)
	defer store.Close()
	backup, err := store.Backup(object.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	directory, _ := saveObjectDirectory(root, object)
	rawPath := filepath.Join(directory, "backups", backup.Name)
	if err = os.Remove(rawPath); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(directory, "active.raw"), rawPath); err != nil {
		t.Fatal(err)
	}
	if err = store.Restore(object.ID, backup.Name); err == nil {
		t.Fatal("symlink backup restored")
	}
	if file, _, openErr := store.OpenDownload(object.ID, backup.Name); openErr == nil {
		file.Close()
		t.Fatal("symlink backup downloaded")
	}
}

func TestSaveDirectorySymlinkTraversalRejected(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "saves")
	outside := t.TempDir()
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "individual")); err != nil {
		t.Fatal(err)
	}
	object := SaveObject{
		ID: "individual:GAME01", Mode: MemoryCardEmulatedIndividual,
		GameID: "GAME01", CardSize: 512 << 10,
	}
	err := EnsureSaveObjects(root, []SaveObject{object}, true, "test", "generation")
	if err == nil || !strings.Contains(err.Error(), "SAVE-CARD-INVALID") {
		t.Fatalf("expected managed-directory rejection, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "GAME01", "active.raw")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("save creation escaped managed root: %v", statErr)
	}
}

func TestConcurrentSaveWritesAndBackupRemainConsistent(t *testing.T) {
	root := t.TempDir()
	object := testSaveObject(t, root)
	store := openTestSaveStore(t, root, object)
	defer store.Close()
	var wait sync.WaitGroup
	errs := make(chan error, 9)
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := store.WriteSaveAt(object.ID,
				bytes.Repeat([]byte{byte(index + 1)}, 512), int64(index*512))
			errs <- err
		}(index)
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		_, err := store.Backup(object.ID, "manual")
		errs <- err
	}()
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Verify(object.ID); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkSaveOverlayWriteAccounting(b *testing.B) {
	root := b.TempDir()
	objects, err := PlanSaveObjects(MemoryCardEmulatedIndividual,
		[]Game{{ID: "GBEN01"}}, "", 16<<20)
	if err != nil {
		b.Fatal(err)
	}
	if err = EnsureSaveObjects(root, objects, true, "benchmark", "generation"); err != nil {
		b.Fatal(err)
	}
	store, err := OpenSaveStore(SaveStoreConfig{
		Root: root, Application: "benchmark", GenerationID: "generation",
		LayoutChecksum: "layout", Metrics: perf.New(perf.Config{Enabled: true}),
	}, objects)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	buffer := bytes.Repeat([]byte{0x61}, 4096)
	b.ReportAllocs()
	b.SetBytes(int64(len(buffer)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		offset := int64(index%1024) * int64(len(buffer))
		if _, err = store.WriteSaveAt(objects[0].ID, buffer, offset); err != nil {
			b.Fatal(err)
		}
	}
}
