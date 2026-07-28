package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"wiibridge/server/host-daemon/gamecube"
	"wiibridge/server/host-daemon/store"
	"wiibridge/shared/model"
	"wiibridge/shared/sourcehealth"
)

func TestUnavailableRootPreservesWiiAndGameCubeCatalogs(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	root := filepath.Join(t.TempDir(), "library")
	if err = os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	preflight, err := sourcehealth.Preflight(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	record := sourcehealth.Successful(preflight.Record, 2)
	if err = database.UpsertSource(record); err != nil {
		t.Fatal(err)
	}
	wiiItems, err := wiiCatalogItems([]model.Game{{
		ID: "KEEP01", Title: "Retained Wii game", Size: 4096,
	}})
	if err != nil {
		t.Fatal(err)
	}
	gameCubeItems, err := gameCubeCatalogItems([]gamecube.Game{{
		ID: "GKEEP1", Title: "Retained GameCube game", Revision: 0,
		Validation: "valid",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.ReconcileCatalogs(map[string][]store.CatalogItem{
		"wii": wiiItems, "gamecube": gameCubeItems,
	}, 2); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(root); err != nil {
		t.Fatal(err)
	}

	wii, offline, scanErr := scanWiiCatalog(database, root)
	if scanErr == nil || len(wii.Games) != 1 ||
		wii.Games[0].Availability != string(sourcehealth.AvailabilitySourceOffline) {
		t.Fatalf("wii=%#v source=%#v err=%v", wii, offline, scanErr)
	}
	gameCubeResult, _, gameCubeErr := scanGameCubeCatalog(database, root, offline)
	if gameCubeErr == nil || len(gameCubeResult.Games) != 1 ||
		gameCubeResult.Games[0].Availability !=
			string(sourcehealth.AvailabilitySourceOffline) {
		t.Fatalf("gamecube=%#v err=%v", gameCubeResult, gameCubeErr)
	}
	for _, platform := range []string{"wii", "gamecube"} {
		items, catalogErr := database.Catalog(platform)
		if catalogErr != nil || len(items) != 1 ||
			items[0].Availability != sourcehealth.AvailabilityPlayable {
			t.Fatalf("%s catalog mutated: %#v err=%v", platform, items, catalogErr)
		}
	}
}

func TestLegacySnapshotMakesUnexpectedlyEmptyFirstScanOffline(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.Publish(model.Snapshot{
		SnapshotID: "legacy", CatalogID: "legacy", VirtualDiskSize: 1 << 30,
		MetadataHash: "legacy", Created: time.Now().UTC(),
		Games: []model.Game{{ID: "KEEP03", Title: "Legacy"}},
	}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	result, record, scanErr := scanWiiCatalog(database, root)
	if scanErr == nil || record.FailureCode != "SOURCE-MOUNT-MISSING" ||
		len(result.Games) != 1 || result.Games[0].ID != "KEEP03" ||
		result.Games[0].Availability != string(sourcehealth.AvailabilitySourceOffline) {
		t.Fatalf("result=%#v source=%#v err=%v", result, record, scanErr)
	}
}

func TestPartialTraversalPreservesPriorCompleteCatalog(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission traversal fault cannot be induced as root")
	}
	database, err := store.Open(filepath.Join(t.TempDir(), "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err = os.Mkdir(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
	preflight, err := sourcehealth.Preflight(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	record := sourcehealth.Successful(preflight.Record, 1)
	if err = database.UpsertSource(record); err != nil {
		t.Fatal(err)
	}
	items, _ := wiiCatalogItems([]model.Game{{ID: "KEEP02", Title: "Retained"}})
	if _, err = database.ReconcileCatalog("wii", items, 2); err != nil {
		t.Fatal(err)
	}
	result, partial, scanErr := scanWiiCatalog(database, root)
	if scanErr == nil || partial.FailureCode != "SOURCE-PARTIAL-SCAN" ||
		len(result.Games) != 1 || result.Games[0].ID != "KEEP02" {
		t.Fatalf("result=%#v source=%#v err=%v", result, partial, scanErr)
	}
}

func TestSourceFailureCodesAreBoundedForRateLimiting(t *testing.T) {
	for _, code := range []string{
		"SOURCE-READ-FAILED", "SOURCE-IDENTITY-CHANGED",
		"SOURCE-PERMISSION-DENIED", "SOURCE-MOUNT-MISSING",
	} {
		if got := normalizedSourceFailureCode(code); got != code {
			t.Fatalf("%q normalized to %q", code, got)
		}
	}
	if got := normalizedSourceFailureCode("attacker-controlled-label-" +
		time.Now().Format(time.RFC3339Nano)); got != "SOURCE-READ-FAILED" {
		t.Fatalf("unbounded failure code normalized to %q", got)
	}
	now := time.Now()
	last := make(map[string]time.Time)
	if !shouldRecordSourceFailure(last, "SOURCE-READ-FAILED", now) ||
		shouldRecordSourceFailure(last, "SOURCE-READ-FAILED", now.Add(29*time.Second)) ||
		!shouldRecordSourceFailure(last, "SOURCE-READ-FAILED", now.Add(30*time.Second)) {
		t.Fatal("per-code 30-second source failure limiter is incorrect")
	}
}
