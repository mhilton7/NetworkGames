# Discovered architecture and GameCube path

## Existing Wii path (preserved)

The host scans an immutable library, parses WBFS headers, and builds a
deterministic synthetic MBR/FAT32 catalog. The NBD server exports that virtual
disk over mutual TLS. On the Pi, `nbd-client` pins the export to `/dev/nbd0`;
the privileged helper verifies its mode, and ConfigFS mass storage exposes the
same block device through the Pi Zero W `dwc2` peripheral controller. The Pi
does not mount it. Wii mode remains the startup default and read-only at the
library, NBD, block-device, and USB LUN layers.

Selection flow:

```text
host web/API selection
→ exportprofile.Manager (session-pinned state machine)
→ Wii vdisk or GameCube FAT32 FileBackend
→ TLS NBD
→ Pi /dev/nbd0
→ ConfigFS mass_storage.0
→ direct Pi Zero W-to-Wii USB cable
→ USB Loader GX
```

No HAT, USB hub, hub overlay, or hub service exists or was added.

## GameCube path

`gamecube.Scan` is a read-only adapter for ISO/GCM, uLoader CISO/CSO, and
extracted FST sources. `gamecube.BuildVolume` creates a persistent,
content-addressed, 33 GiB sparse MBR image with one FAT32 LBA partition,
32 KiB clusters, matching primary/backup boot sectors, and the standard
Nintendont `/games/<Title> [ID]/game.iso` plus optional `disc2.iso` layout.

The completed image moves atomically from `cache/.building` to `cache/ready`.
Only a complete manifest can be selected. The GameCube profile uses the same
NBD and USB gadget identity as Wii mode. Physical-card mode remains read-only.
Emulated-card mode enables writes only for the selected GameCube volume.

The manager states are `DISCONNECTED`, `PREPARING`, `VALIDATING`, `EXPORTING`,
`CONNECTED`, `ACTIVE`, `DISCONNECTING`, `RECOVERY_REQUIRED`, and `ERROR`.
Selection is rejected while any NBD session is active. A profile is validated
before publication, and its backend stays pinned until that session ends.

## Persistence and deployment

- Source games: existing read-only `/library` bind mount.
- Host database and snapshots: `/data`.
- GameCube cache: `/data/gamecube/cache`.
- Per-title settings: `/data/gamecube/settings`.
- Save history: `/data/gamecube/save-backups`.
- Host runtime: scratch OCI image.
- Pi runtime: systemd controller plus typed privileged helper and ConfigFS
  script packaged by pi-gen.

The implementation does not change the source game, Wii extent mapping, USB
VID/PID, serial derivation, manufacturer, product string, or UDC choice.
