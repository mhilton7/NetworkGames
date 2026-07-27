# GameCube save safety

Physical-card mode leaves the complete NBD and USB storage path read-only.
Emulated mode is explicitly selected at the host and Pi; only then are the
NBD block device and mass-storage LUN writable.

On a normal switch back to Wii mode, the host closes the GameCube backend,
validates the FAT volume, reads `.raw` files without mounting, rejects
zero-byte or unsupported sizes, and writes atomic timestamped backups under:

```text
/data/gamecube/save-backups/<GAMEID>/r<revision>/<name.raw>/
```

At least five versions are retained per card. Temporary files are never
listed as restore points. Restore accepts only a path from the managed backup
inventory, validates its size, preserves the current card first, writes only
the named `/saves/*.raw` file while detached, and revalidates the volume.

Supported card-file sizes are 512 KiB, 1, 2, 4, 8, and 16 MiB. IDs and
revisions have separate settings and backup trees. Nintendont's standard
per-game naming (first four ID characters) or region-specific multi-card name
is preserved; the project does not merge cards between identities.

The utilities are:

```sh
go run ./scripts/gamecube-saves backup \
  -manifest /data/gamecube/cache/ready/<key>/manifest.json \
  -backups /data/gamecube/save-backups -retain 5

go run ./scripts/gamecube-saves restore \
  -manifest /data/gamecube/cache/ready/<key>/manifest.json \
  -backup /data/gamecube/save-backups/<managed-file>.raw \
  -name GAME.raw
```

Both commands require the gadget detached and NBD disconnected. The web
restore action enforces the same session condition.
