# GameCube support

GameCube mode is a separate, complete Nintendont-compatible FAT32 library
export. Wii mode remains the startup default and retains its existing read-only
virtual disk unchanged.
The Pi remains connected directly to the Wii gadget port; HATs and USB hubs
are unsupported.

## Supported sources

- `.iso` and `.gcm` with a valid GameCube header and readable FST.
- Nintendont/uLoader `.ciso` and `.cso` with the pinned 2 MiB mapping format.
- Extracted FST games containing valid `sys/boot.bin`, `sys/bi2.bin`,
  `sys/apploader.img`, `sys/main.dol`, and `files/`.
- One- and two-disc image sets paired by header metadata.

NKit, RVZ, ZIP, arbitrary compression, and mixed/ambiguous sets are rejected.
Sources are scanned and hashed read-only. They become playable only after a
validated metadata generation is complete.

## Operator workflow

1. Put a legal GameCube backup in the existing read-only library dataset.
2. In the Host UI choose **Build GameCube Library**. The Host creates and
   validates an inactive generation containing every validated title.
3. Click **Activate GameCube Library**. The Host performs the complete safe
   detach/disconnect/select/connect/attach sequence.
4. Start USB Loader GX and browse the complete GameCube catalog.
5. Click **Activate Wii Library** to return to the complete Wii catalog.

Never mount the exported GameCube block device on the host or Pi while the Wii
owns it.

## Managed generations

Generated data is stored under `/data/gamecube/library`:

```text
active.json
generations/<generation-id>/manifest.json
generations/<generation-id>/layout.bin
generations/<generation-id>/metadata.bin
generations/<generation-id>/checksums.json
generations/<generation-id>/validation.json
```

Builds use `.building-*` staging directories, validate the closed metadata and
source map, rename the generation atomically, and then atomically update
`active.json`. A failed or canceled build cannot replace the prior generation.
Source changes show **Update available** and invalidate activation.

`validation.json` is a compact receipt for a completed deep source validation.
Startup first checks only the schema, layout and metadata hashes, extent
geometry, managed paths, regular-file type, and stored size/device/inode/mtime
identities. It never hashes every ISO before opening HTTPS or before making the
Wii export ready. If the receipt is absent or stale, full SHA-256 validation
runs as a cancellable background operation; GameCube remains unavailable until
it succeeds. The receipt is not trusted after any recorded source identity or
generation checksum changes.

The apparent virtual FAT32 disk includes the mapped size of every payload,
headroom, and save reserve. Physical `/data` usage contains only FAT32 metadata,
the extent map, manifests, checksums, and optional saves. ISO, GCM, CISO, and FST
payload bytes remain only under the read-only `/library` mount. Retained
schema-2 generations therefore do not multiply payload storage.

Runtime extent lookup uses virtual-offset-sorted mappings and binary search.
The Host retains at most 32 read-only source handles in an LRU cache, rechecks
bounded file identity before source reads, coalesces reads within one extent,
and closes every cached handle when the export profile closes. It does not hash
whole games, walk source directories, or parse generation JSON on the NBD read
path.

Schema-1 generations containing `library.img` are detected but never removed
automatically. After a schema-2 generation is built and validated, return to
Wii mode, detach USB, disconnect NBD, stop the Host, and remove only the
validated legacy generation directory beneath
`/data/gamecube/library/generations`. Preserve save backups and never follow
symlinks during cleanup.

Memory-card modes are `physical`, `emulated-individual`, and
`emulated-shared`. Physical mode preserves the fully read-only export.
Individual mode maps one managed raw card per Game ID; shared mode maps only
the explicitly selected named card. Emulated modes authorize writes solely
through the immutable Host-generated save-extent map. They never merge cards,
modify a game source, or fall back to a copied image. Full storage, bounds,
durability, backup, restore, and recovery details are in
[gamecube-save-overlay.md](gamecube-save-overlay.md).

## USB Loader GX / Nintendont settings

- GameCube loader: Nintendont.
- Nintendont executable: `sd:/apps/Nintendont/boot.dol`.
- GameCube source/path: USB, `/games`.
- Video mode: Auto or Disc Default initially.
- Optional patches, widescreen, progressive, cheats, and IPL: off/auto unless
  a per-title test justifies them.
- Native controller: enable only for compatible original Wii hardware.
- Memory card: match the selected host/Pi mode.
- Wii loader/game cIOS settings: unchanged.

Copy `dist/wii-sd/apps/Nintendont` to the same path on the Wii SD card only
after verifying the hashes in `diagnostics/gamecube/upstream-integration.md`.
No channel or NAND modification is performed.

## Host utilities

```sh
go run ./scripts/gamecube-volume scan -library /path/to/read-only/library
go run ./scripts/gamecube-volume build \
  -library /path/to/read-only/library \
  -cache /path/to/persistent/cache -id GAMEID -revision 0 \
  -memory-card physical
go run ./scripts/gamecube-volume validate \
  -manifest /path/to/cache/ready/<key>/manifest.json
```

Save backup/restore commands are in
`diagnostics/gamecube/save-safety.md`. Hardware results belong only in
`diagnostics/gamecube/hardware-validation.md`.
