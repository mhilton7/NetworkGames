# Backup and restore

Snapshot `/config`, `/data`, and `/backups` consistently while the app is
stopped or after requesting a quiescent diagnostic export. Back up certificates
through the organization's secret-management procedure; do not put private keys
in diagnostics. `/library` is not copied by this application.

To restore, recreate datasets and ACLs, restore compact metadata, install the
same immutable image digest, and scan sources. A source identity mismatch makes
the affected old snapshot unhealthy and requires a new snapshot.
