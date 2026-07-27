# GameCube storage design

The existing Wii export is synthetic and unsuitable as a Nintendont game
filesystem, so GameCube uses a dedicated cached FAT32 disk image.

Properties:

- DOS/MBR partition table, FAT32 LBA type, first partition at LBA 2048.
- 33 GiB stable virtual capacity and one primary partition.
- 512-byte logical sectors and 32 KiB FAT clusters.
- Valid backup boot sector with matching geometry (63 sectors/track,
  255 heads, 2048 hidden sectors).
- `/games`, `/saves`, `/controllers`, and `/apps/Nintendont` directories.
- Content-addressed key includes schema, game ID/revision, card mode, disc
  number, format, and source SHA-256.
- Temporary construction under `.building`; atomic rename to `ready`.
- Complete manifest and structural FAT validation required before selection.

Single-disc files become `game.iso` (or `game.ciso`). Two validated discs with
the same ID, region, revision, and distinct disc numbers become `game.iso` and
`disc2.iso`. Pairing never relies on filenames. Extracted FST is copied with
its directory structure and is limited to a single-disc layout.

The source library is never modified. A selected network source is fully
copied into the persistent cache at import time; launch never depends on live
decompression or a partial build. The Pi never mounts the image. Platform
switching requires USB detach and NBD disconnect, then host selection,
reconnection in the matching Pi mode, and gadget attachment.

The volume validator is implemented in pure Go because the scratch host image
cannot execute `fsck.fat`. It validates MBR signature/type/start, FAT32 boot
geometry, backup-boot equality, capacity, and every required path. An external
`fsck.fat -n` remains an additional release validation when `dosfstools` is
available in a non-production validation environment.
