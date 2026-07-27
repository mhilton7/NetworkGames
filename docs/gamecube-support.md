# GameCube support

GameCube mode is a separate Nintendont-compatible FAT32 export. Wii mode is
still the startup default and retains its existing read-only virtual disk.
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
2. In the host UI choose GameCube, verify ID/region/revision/status, configure
   per-game settings, and click Import once.
3. Detach USB and disconnect NBD on the Pi.
4. Select the ready GameCube entry on the host.
5. On the Pi choose either:
   - `Connect GameCube (physical card, read-only)`, or
   - `Connect GameCube (emulated card, saves writable)`.
6. Attach USB, start USB Loader GX, and launch through Nintendont.
7. Detach/disconnect before switching. Selecting Wii on the host performs the
   emulated-card backup and restores the normal read-only Wii export.

Never mount the GameCube image on the host or Pi while exported.

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
