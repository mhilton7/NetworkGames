# GameCube feature baseline

Captured 2026-07-26 PDT before GameCube source edits.

## Source and rollback

- Historical checkout path at capture: `/redacted/local/NetworkGames-zero-w-boot-fix`
- Starting branch: `fix/squeakquel-and-io-performance`
- Starting commit: `64ff84b43700d96c0ba7ab495a006301a9ff2014`
- Feature branch: `feature/gamecube-support`
- Known-working production ancestor: `f3a2cb2158819b640936d050ba3529bad4ddb7c1`
- Full pre-feature archive:
  `/redacted/local/NetworkGames-zero-w-boot-fix-full-backup-20260726-202721.tar.zst`
- Archive SHA-256:
  `18c0e7f2897c6f4b3afbc3a9947a4c28fab552f98f7483376849385505b8e332`
- Archive integrity: `zstd -t` and `sha256sum -c` passed.

Pre-existing worktree state deliberately remains outside feature commits:

```text
M reports/firmware/zero-w-armhf/networkgames-hostbridge-0.1.0-rc.1-zero-w-armhf.offline-validation.json
M reports/firmware/zero-w-armhf/networkgames-hostbridge-0.1.0-rc.1-zero-w-armhf.provenance.json
?? CODEX_HOST_RESTORE_REPORT.md
?? deploy/truenas/compose.ghcr.yaml
```

## Running production path

- TrueNAS host image:
  `ghcr.io/OWNER/networkgames-host:0.1.0-rc.1-squeakquel-io@sha256:ded61132346b902b8e74e966bb1a97fe5b428a8ffce68d9f9e050faabaf14bbc`
- Host health: `{"status":"healthy","version":"0.1.0-rc.1"}`
- Pi target: `zero-w-armhf`
- Detected board: `Raspberry Pi Zero W Rev 1.1`
- Pi controller state: `ready`
- NBD: connected, mutual TLS, read-only
- USB gadget: ConfigFS `mass_storage.0` on `20980000.usb`
- USB link at capture: `configured`
- Direct Pi-to-Wii USB cable; no HAT or hub is present or supported.
- Boot configuration: `dtoverlay=dwc2,dr_mode=peripheral` and boot-loaded
  `dwc2`; `nbd` is preloaded with `nbds_max=1`.
- Game LUN: `/dev/nbd0`, `lun.0/ro=1`, backing block device verified
  read-only before attachment.
- Existing Wii export: deterministic MBR plus synthetic FAT32 metadata with
  `/wbfs/*.wbfs` payload extents mapped directly to the read-only library.
- The Pi does not mount the Wii export during gadget ownership.
- Root filesystem is ordinary ext4 with `noatime`; no OverlayFS,
  overlayroot, volatile root, or read-only-root conversion is enabled.

## Physical Wii evidence

- `SAVE5G` Squeakquel: catalog, animated banner, launch and initial gameplay
  passed on the physical Wii through this exact production path.
- 10 Minute Solution: banner and launch passed after the shared FAT attribute
  correction.
- A new two-title regression run belongs to the pre-deployment GameCube
  hardware gate; no result is inferred from older evidence.

## Baseline commands and results

```sh
sha256sum -c /redacted/local/NetworkGames-zero-w-boot-fix-full-backup-20260726-202721.tar.zst.sha256
make test
make static
./deploy/truenas/validate-compose.sh
curl -sk https://192.0.2.10:9443/healthz
curl --cacert /path/to/ca.crt https://192.0.2.10:8445/healthz
```

Results:

- Backup checksum: pass.
- `make test`: pass.
- `make static`: pass.
- Compose policy and Docker Compose parser: pass.
- Pi and TrueNAS health: pass.

An initial accidental invocation of `make test`, `make static`, and Compose
validation from `/redacted/local` failed because no project Makefile exists there.
The commands were immediately rerun from the checkout and passed; no source
or system state was changed by the mistaken invocation.

## Upstream pins captured for implementation

- Nintendont official repository: `FIX94/Nintendont`
- Nintendont commit:
  `0f69235b99099ef2851a5f6b7b0e349c572237e8`
- USB Loader GX official repository: `wiidev/usbloadergx`
- USB Loader GX commit/version:
  `e25c4f3501ed957b7db73f79c51fdf00715ab2e2`, r1283

Detailed source findings are recorded separately in
`upstream-integration.md`.
