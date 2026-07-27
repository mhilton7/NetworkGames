# GameCube deployment

The primary Host export is a complete synthetic library, not a per-title
volume. Game payloads remain in the read-only `/library` dataset. `/data` needs
space only for FAT32 metadata, extent maps, manifests, checksums, temporary
metadata staging, and saves. Build with **Build GameCube Library**, inspect its
title/disc/file counts, then use **Activate GameCube Library**.

```text
WIIBRIDGE_GAMECUBE_MEMORY_CARD_MODE=physical
WIIBRIDGE_GAMECUBE_HEADROOM_PERCENT=5
WIIBRIDGE_GAMECUBE_SAVE_RESERVE_MIB=1024
WIIBRIDGE_GAMECUBE_MAX_VOLUME_GIB=
WIIBRIDGE_GAMECUBE_GENERATION_RETENTION=2
```

`physical` exports read-only and is the currently supported no-copy mode.
`emulated` is rejected with a clear startup error until a bounded, save-only
copy-on-write overlay is available. It does not silently fall back to physical
mode or to a full copied image.

Deployment is intentionally not performed by the build. Obtain explicit
operator approval after reviewing the diff and host-test report.

## Build and verify

From the repository root:

```sh
make test
make static
make compose
make oci
sha256sum dist/wii-sd/apps/Nintendont/*
git diff ada2b010900c348eb9504c3eeb71b6fd10cb7e65..HEAD
```

Record the generated OCI digest and Pi image checksum. Snapshot the TrueNAS
config/data/backups datasets and retain the current Pi SD image before any
change.

## Host update

Follow `deploy/truenas/upgrade.md`: with USB detached and NBD disconnected,
replace only `WIIBRIDGE_IMAGE` with the newly built/imported immutable
digest. Do not change certificates, tokens, paths, UID/GID, USB identity, or
the read-only library mount.

Virtual capacity is calculated from mapped payload sizes, configured headroom,
and save reserve. This apparent size is not allocated under `/data`. Updates
stage and retain only compact metadata, so retained generations do not duplicate
the source library. Set `WIIBRIDGE_GAMECUBE_MAX_VOLUME_GIB` if the deployed
NBD/kernel combination has a verified lower capacity limit; the build then
fails clearly instead of truncating the virtual library.

## Pi image

This Host change reuses the deployed typed `connect-wii`,
`connect-gamecube-physical`, `connect-gamecube-emulated`, `detach`,
`disconnect`, and `attach` actions; it does not require a firmware reflash when
those actions are already present. If a separate verified firmware update is
ever required, follow `docs/flashing.md`, preserve the existing card as the
rollback image, and retain this device's matching credentials and VID/PID.

## Wii SD package

With the SD card mounted on an operator workstation and backed up:

```sh
cp -a dist/wii-sd/apps/Nintendont /path/to/wii-sd/apps/
sha256sum /path/to/wii-sd/apps/Nintendont/*
```

This writes only Homebrew Channel files. It does not write Wii NAND.

## Exact platform switch sequence

After host and Pi deployment:

```sh
# Pi: detach first and end the NBD session.
sudo /usr/libexec/wiibridge-helper detach
sudo /usr/libexec/wiibridge-helper disconnect

# Host API: build the complete library in the background when needed.
curl --cacert /path/to/host-ca.crt -u "admin:$WIIBRIDGE_ADMIN_TOKEN" \
  -H 'X-WiiBridge-CSRF: 1' -X POST \
  https://HOST:8445/api/v1/gamecube/library/build

# After GET /api/v1/gamecube/library reports Ready, select the synthetic library.
curl --cacert /path/to/host-ca.crt -u "admin:$WIIBRIDGE_ADMIN_TOKEN" \
  -H 'X-WiiBridge-CSRF: 1' -X POST \
  https://HOST:8445/api/v1/export/gamecube

# Pi: choose exactly one matching card mode, then attach.
sudo /usr/libexec/wiibridge-helper connect-gamecube-physical
sudo /usr/libexec/wiibridge-helper attach
```

To return to Wii mode, detach/disconnect, POST `/api/v1/export/wii`, then:

```sh
sudo /usr/libexec/wiibridge-helper connect-wii
sudo /usr/libexec/wiibridge-helper attach
```

Proceed through `diagnostics/gamecube/hardware-validation.md`. Do not declare
production acceptance until all physical gates pass.

## Reclaiming legacy copied generations

If the dashboard reports **Legacy copied GameCube generation detected**, first
build and validate a new schema-2 generation. Switch to Wii, detach USB,
disconnect NBD, and stop the Host. Inspect each candidate directory under
`/data/gamecube/library/generations`; a legacy directory contains schema 1
`manifest.json` and/or `library.img`. Back up save data, then remove only the
confirmed legacy directory. Do not use a wildcard, do not follow symlinks, and
do not remove `active.json`, authentication data, or
`/data/gamecube/save-backups`.
