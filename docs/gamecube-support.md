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
validated persistent cache is complete.

## Operator workflow

1. Put a legal GameCube backup in the existing read-only library dataset.
2. In the Host UI choose **Build GameCube Library**. The Host creates and
   validates an inactive generation containing every validated title.
3. Click **Activate GameCube Library**. The Host performs the complete safe
   detach/disconnect/select/connect/attach sequence.
4. Start USB Loader GX and browse the complete GameCube catalog.
5. Click **Activate Wii Library** to return to the complete Wii catalog.

Never mount the GameCube image on the host or Pi while exported.

## Managed generations

Generated data is stored under `/data/gamecube/library`:

```text
active.json
generations/<generation-id>/library.img
generations/<generation-id>/manifest.json
```

Builds use `.building-*` staging directories, validate the closed image, rename
the generation atomically, and then atomically update `active.json`. A failed
build cannot replace the prior generation. Source changes show **Update
available** and never modify an open backend.

Sizing includes all prepared payloads, FAT metadata, configurable headroom, and
save reserve. Plan writable Host storage for roughly the physical size of all
supported GameCube sources, plus at least 5% and 1 GiB. Source files remain on
the read-only library mount and are never modified.

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
