package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wiibridge/shared/model"
	"wiibridge/shared/perf"
	"wiibridge/shared/sourcehealth"
)

func testCatalogItem(t *testing.T, id string) CatalogItem {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"id": id, "title": "Synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	return CatalogItem{ID: id, Payload: payload}
}

func TestOfflineSourcePreservesCatalogAndStateAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record := sourcehealth.Successful(sourcehealth.Record{
		SourceID: "source-1", RootPath: "/library",
		State: sourcehealth.StateAvailable, LastAttemptedScan: time.Now().UTC(),
	}, 1)
	if err = database.UpsertSource(record); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ReconcileCatalog(
		"wii", []CatalogItem{testCatalogItem(t, "GAME01")}, 2); err != nil {
		t.Fatal(err)
	}
	offline := sourcehealth.RuntimeFailure(record, "SOURCE-READ-FAILED")
	if err = database.UpsertSource(offline); err != nil {
		t.Fatal(err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	items, err := database.Catalog("wii")
	if err != nil || len(items) != 1 ||
		items[0].Availability != sourcehealth.AvailabilityPlayable {
		t.Fatalf("catalog=%#v err=%v", items, err)
	}
	persisted, err := database.SourceByRoot("/library")
	if err != nil || persisted.State != sourcehealth.StateTemporaryUnavailable ||
		persisted.LastSuccessfulItemCount != 1 {
		t.Fatalf("source=%#v err=%v", persisted, err)
	}
}

func TestConfirmedDeletionRequiresTwoCompleteReconciliations(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err = database.ReconcileCatalog(
		"gamecube", []CatalogItem{testCatalogItem(t, "GTEST0:r0")}, 2); err != nil {
		t.Fatal(err)
	}
	first, err := database.ReconcileCatalog("gamecube", nil, 2)
	if err != nil || len(first) != 1 ||
		first[0].Availability != sourcehealth.AvailabilityValidationRequired {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := database.ReconcileCatalog("gamecube", nil, 2)
	if err != nil || len(second) != 1 ||
		second[0].Availability != sourcehealth.AvailabilityMissingConfirmed ||
		second[0].MissingConfirmed.IsZero() {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	if err = database.AcknowledgeMissing("gamecube", "GTEST0:r0"); err != nil {
		t.Fatal(err)
	}
	if items, catalogErr := database.Catalog("gamecube"); catalogErr != nil || len(items) != 0 {
		t.Fatalf("acknowledged catalog=%#v err=%v", items, catalogErr)
	}
}

func TestPerformanceSessionPersistenceIsBounded(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for index := 0; index < 4; index++ {
		summary := perf.SessionSummary{
			ID: string(rune('a' + index)), Start: time.Now().UTC(),
			End:      time.Now().UTC().Add(time.Duration(index) * time.Second),
			Platform: "wii", Outcome: "detached",
		}
		if err = database.SavePerformanceSession(summary, 2, 30); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := database.PerformanceSessions(100)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("sessions=%#v err=%v", sessions, err)
	}
}

func TestSchema2MigrationSeedsLegacyWiiSnapshotSafely(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite3")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.Snapshot{
		SnapshotID: "legacy-snapshot", CatalogID: "legacy-catalog",
		VirtualDiskSize: 1 << 30, MetadataHash: "legacy",
		Application: "legacy", Created: time.Now().UTC(),
		Games: []model.Game{{ID: "KEEP01", Title: "Legacy Wii game", Size: 4096}},
	}
	if err = database.Publish(snapshot); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"DELETE FROM schema_migrations WHERE version=2",
		"DROP TABLE source_roots", "DROP TABLE catalog_items",
		"DROP TABLE source_events", "DROP TABLE compatibility_cache",
		"DROP TABLE performance_sessions",
	} {
		if _, err = database.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	items, err := database.Catalog("wii")
	if err != nil || len(items) != 1 || items[0].ID != "KEEP01" ||
		items[0].Availability != sourcehealth.AvailabilityPlayable {
		t.Fatalf("migrated catalog=%#v err=%v", items, err)
	}
	info, err := os.Lstat(path + ".pre-schema2.bak")
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("schema-2 rollback backup=%v err=%v", info, err)
	}
}
