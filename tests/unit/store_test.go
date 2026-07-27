package unit_test

import (
	"path/filepath"
	"testing"
	"time"

	"wiibridge/server/host-daemon/store"
	"wiibridge/shared/model"
)

func TestSQLiteMigrationAndAtomicPublish(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := model.Snapshot{SnapshotID: "immutable-1", CatalogID: "all",
		VirtualDiskSize: 1234, MetadataHash: "abc", Application: "test", Created: time.Now().UTC()}
	if err := db.Publish(want); err != nil {
		t.Fatal(err)
	}
	got, err := db.Active()
	if err != nil {
		t.Fatal(err)
	}
	if got.SnapshotID != want.SnapshotID || got.VirtualDiskSize != want.VirtualDiskSize {
		t.Fatalf("got %#v", got)
	}
}
