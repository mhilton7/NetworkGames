# GameCube save-only overlay

WiiBridge save-overlay format 1 adds writes to the schema-2 GameCube no-copy
export without making the game library writable. It supports:

- `physical`: unchanged, fully read-only export; the Wii uses a physical card.
- `emulated-individual`: one managed raw card per six-character Game ID.
- `emulated-shared`: one explicitly named card shared by the selected library.

Changing modes never merges cards and a missing or invalid card never causes a
fallback to another mode. The legacy ambiguous value `emulated` remains
rejected. Emulated activation also requires a fresh Pi negotiation containing
`gamecube-save-overlay-v1`.

## Storage and format

Authoritative state is below `/data/gamecube/saves`, never a cache:

```text
individual/<GAMEID>/
shared/<card-name>/
    active.raw
    metadata.json
    journal/pending.log
    backups/<timestamp>-<checksum>.raw
    backups/<timestamp>-<checksum>.json
```

The leaf layout is the same for individual and shared cards. Names are
server-generated from validated identifiers. Symlinks, path traversal, and
special files are rejected. Supported card sizes are exactly 512 KiB, 1 MiB,
2 MiB, 4 MiB, 8 MiB, and 16 MiB. A newly created card is a fixed-size blank raw
card for Nintendont to initialize; Host validation checks association, regular
file type, exact size, metadata, and SHA-256. It does not claim that an
arbitrary upload contains a fully valid GameCube filesystem.

Format-1 metadata binds a card to its mode and Game ID or shared-card name,
card size, application version, generation, checksum, last flush/backup,
dirty/recovery state, and file identity. Unknown older/newer formats, wrong
association, wrong size, checksum mismatch, or ambiguous recovery block the
affected emulated GameCube export. Wii and physical-card mode remain usable.

## Trusted writable extents

The Host FAT32 builder allocates a raw save file and emits a checksummed,
host-generated extent containing:

```text
virtual offset and length
save object ID
card file offset and size
generation ID
layout checksum
```

The generation manifest, layout, metadata, and save-extent hashes are validated
before use. Guest FAT tables and directory entries are never consulted for
write authorization. Every NBD write must fit completely inside one extent.
Writes to game data, FATs, directories, boot sectors, padding, free space, or
across the extent boundary return a protocol error. No write is ever forwarded
to ISO, GCM, CISO, CSO, FST, or WBFS files.

## Bounded journal and checkpoints

Writes are journaled as offset, length, SHA-256, and bytes, then synced before
the corresponding fixed 512-byte dirty blocks are exposed to subsequent reads.
Defaults and hard limits are:

| Bound | Value |
|---|---:|
| Card/upload size | 16 MiB |
| Pending bytes | 16 MiB |
| Dirty blocks | 32,768 (16 MiB) |
| Journal | 64 MiB |
| Backups | configurable 1–100; default 5 |

A request that cannot fit those bounds fails with `SAVE-JOURNAL-LIMIT`.
Accumulated state is checkpointed when necessary; dirty data is not silently
dropped. Dirty blocks, journal bytes, flushes, failures, recovery, backups,
restores, and rejected writes are observable as counters only—save contents
are never telemetry.

Checkpointing copies only the bounded raw save card into a same-directory
temporary file, applies dirty blocks, syncs it, verifies it, records a
hard-link/copy of the previous confirmed card, atomically renames the candidate,
writes and syncs metadata, verifies the reopened card, and removes the rollback
marker. The journal is truncated only after confirmation. Startup replays a
complete journal and rolls back an interrupted activation to the previous
provably valid card. Conflicting staged candidates or an incomplete/corrupt
journal block emulated mode with `SAVE-RECOVERY-AMBIGUOUS`.

These rules provide software crash consistency when filesystem rename, file
sync, and directory sync behave as specified. They do not prove survival of
storage-controller failure, missing write barriers, flash wear, or power loss
outside the filesystem/hardware guarantees.

## Backup, restore, upload, and download

Backups are complete bounded card copies plus format-1 metadata. A backup is
synced and validated before older pairs are pruned; retention is never below
one. Cleanup accepts only managed regular-file pairs.

Restore and upload require the GameCube export to be inactive. Restore
validates a selected managed backup, stages it before pruning could affect it,
flushes pending writes, creates a `pre-restore` safety backup, activates the
staged candidate transactionally, reopens it, and verifies SHA-256. Upload
ignores the client filename, enforces the selected card size and 16 MiB limit,
stages raw bytes, creates a `pre-upload` backup, and follows the same activation
path. Archives are not accepted. Authenticated download streams only the
selected managed active card or backup using a generated filename.

The dashboard exposes mode/selection, size, integrity, dirty/recovery state,
last flush/backup, backup history, checksum, create, verify, backup, restore,
upload, and download. All mutations retain browser authentication, CSRF, and
confirmation conventions.

## Rollback

Safely detach USB, disconnect NBD, stop the Host, and back up
`/data/gamecube/saves` plus `/data/gamecube/save-settings.json`. Restoring an
older Host binary leaves format-1 cards intact, but older software must be
configured for `physical` because it cannot authorize format-1 extents. Never
move cards into `/library` and never remove a `.previous-confirmed.*` marker or
journal by hand; start the new Host and use its recovery status.
