# Squeakquel path inventory

Captured 2026-07-26 PDT before source edits.

## Repository and rollback baseline

- Historical active checkout: `[redacted local path]`
- Starting branch: `agent/fix-pi-provisioning-nbd-idle`
- Starting commit: `f3a2cb2158819b640936d050ba3529bad4ddb7c1`
- Working branch: `fix/squeakquel-and-io-performance`
- Known-working source/backup: commit `f3a2cb2` and a verified local backup
- Pre-restore archive retained at `[redacted local path]`; SHA-256
  `73c34f9a553112474cbcc6180545ba564ba9a77d9e02076a1db104cc0b72c0a3`.
- Existing user changes were present before this task and are excluded from
  every task commit: two modified Zero W report JSON files plus untracked
  `CODEX_HOST_RESTORE_REPORT.md` and `deploy/truenas/compose.ghcr.yaml`.
- Remote `origin` is a locally mounted backup checkout, not GitHub.
- Available space at capture: 37 GiB; local device details redacted.

## Actual data path

`USB Loader GX r1283 -> Wii USB controller -> USB cable -> Pi Zero W dwc2
peripheral controller -> Linux ConfigFS mass_storage.0 -> read-only /dev/nbd0
-> TLS NBD export "all" -> synthesized MBR/FAT32 virtual disk -> extent mapper
-> read-only WBFS source library`

The host daemon synthesizes only filesystem metadata. WBFS payload bytes are
served directly from immutable source extents. The Pi does not parse WBFS.
The Linux mass-storage gadget owns SCSI handling; the project supplies a
read-only block device rather than a userspace FunctionFS SCSI target.

## Host and live service

- Local validation-host details: redacted.
- Live TrueNAS endpoint: private address redacted.
- Strict TLS health at port 8445 returned
  `{"status":"healthy","version":"0.1.0-rc.1"}`.
- Mutual-TLS NBD at port 10809 exported `all`, read-only, with stable size
  10,967,487,488 bytes during this capture.
- The library is exposed inside the host container as `/library:ro`. The
  TrueNAS dataset's host path was not obtainable without TrueNAS admin/SSH
  access and is therefore not guessed.
- NBD transmission negotiation clears its deadline before normal reads.

## Pi media and firmware

The Pi was not running: its SD card was attached to this host as a confirmed
removable device (local device path redacted),
so live `uname`, `systemctl`, `journalctl`, `dmesg`, `vcgencmd`, `lsusb -t`
and runtime throttling data could not be collected.

Read-only inspection of the card showed:

- Target: Raspberry Pi Zero W, `zero-w-armhf`.
- OS: Raspbian 13 (trixie).
- Kernel modules: `6.18.34+rpt-rpi-v6` (with v7/v8 module trees packaged).
- Boot: `dtoverlay=dwc2,dr_mode=peripheral`; command line loads `dwc2`.
- Root: ordinary writable ext4 with `defaults,noatime`; no overlayroot,
  OverlayFS root, volatile root, or read-only-root conversion.
- NBD packages: libnbd 1.22.2-1+b2 and nbd-client 3.26.1.
- NetworkManager 1.52.1, hostapd 2.10, dnsmasq 2.91.
- `nbd` is boot-loaded with `nbds_max=1`.
- Gadget helper requires `/dev/nbd0` to be read-only, creates
  `functions/mass_storage.0`, sets `lun.0/ro=1`, and uses the configured
  VID/PID without inventing defaults.
- Involved services: controller, auto-attach, recovery, setup AP, hostapd,
  dnsmasq, first-boot and system module loading.
- Journald is persistent and bounded to 32 MiB.

The attached image was built from the known-working `f3a2cb2` tree. It was
not booted or modified in this task. A live deployed Pi firmware version
could not be queried while the card was attached.

## Background services and storage

Normal gameplay retains Wi-Fi, NetworkManager, controller and recovery
services. No service was disabled for benchmark results. The payload is
network-backed, uncompressed WBFS in a read-only library, dynamically mapped
into a synthesized FAT32 disk. The current `SAVE5G.wbfs` entry is contiguous
in the synthesized view and is neither split nor sparse at the exported-file
level.
