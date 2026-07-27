// Package store owns the versioned SQLite control-plane database.
package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"wiibridge/shared/model"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_utc TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS snapshots(
 snapshot_id TEXT PRIMARY KEY, catalog_id TEXT NOT NULL,
 virtual_disk_size INTEGER NOT NULL, metadata_hash TEXT NOT NULL,
 manifest_json BLOB NOT NULL, healthy INTEGER NOT NULL DEFAULT 1,
 created_utc TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS active_snapshot(
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 snapshot_id TEXT NOT NULL REFERENCES snapshots(snapshot_id));
CREATE TABLE IF NOT EXISTS audit_events(
 id INTEGER PRIMARY KEY AUTOINCREMENT, event_type TEXT NOT NULL,
 detail TEXT NOT NULL, created_utc TEXT NOT NULL);
INSERT OR IGNORE INTO schema_migrations(version,applied_utc)
VALUES(1,strftime('%Y-%m-%dT%H:%M:%fZ','now'));`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) Publish(snapshot model.Snapshot) error {
	manifest, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT OR IGNORE INTO snapshots
 (snapshot_id,catalog_id,virtual_disk_size,metadata_hash,manifest_json,created_utc)
 VALUES(?,?,?,?,?,?)`, snapshot.SnapshotID, snapshot.CatalogID,
		snapshot.VirtualDiskSize, snapshot.MetadataHash, manifest,
		snapshot.Created.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO active_snapshot(singleton,snapshot_id) VALUES(1,?)
 ON CONFLICT(singleton) DO UPDATE SET snapshot_id=excluded.snapshot_id`,
		snapshot.SnapshotID); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO audit_events(event_type,detail,created_utc)
 VALUES('snapshot_published',?,?)`, snapshot.SnapshotID,
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Active() (model.Snapshot, error) {
	var manifest []byte
	err := s.db.QueryRow(`SELECT s.manifest_json FROM snapshots s
 JOIN active_snapshot a ON a.snapshot_id=s.snapshot_id
 WHERE a.singleton=1 AND s.healthy=1`).Scan(&manifest)
	if err != nil {
		return model.Snapshot{}, err
	}
	var snapshot model.Snapshot
	err = json.Unmarshal(manifest, &snapshot)
	return snapshot, err
}

func (s *Store) Close() error { return s.db.Close() }
