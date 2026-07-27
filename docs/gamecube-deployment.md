# GameCube deployment

Deployment is intentionally not performed by the build. Obtain explicit
operator approval after reviewing the diff and host-test report.

## Build and verify

From the repository root:

```sh
make test
make static
make compose
make oci
make firmware-zero-w
sha256sum dist/wii-sd/apps/Nintendont/*
git diff 64ff84b43700d96c0ba7ab495a006301a9ff2014..HEAD
```

Record the generated OCI digest and Pi image checksum. Snapshot the TrueNAS
config/data/backups datasets and retain the current Pi SD image before any
change.

## Host update

Follow `deploy/truenas/upgrade.md`: with USB detached and NBD disconnected,
replace only `WIIBRIDGE_IMAGE` with the newly built/imported immutable
digest. Do not change certificates, tokens, paths, UID/GID, USB identity, or
the read-only library mount. Confirm `/data` has at least 35 GiB free per
concurrently cached title plus save history and snapshot headroom.

## Pi image

The Pi helper must be updated for explicit writable GameCube save mode. Flash
only a separately verified removable microSD after approval and exact device
inspection, following `docs/flashing.md`; preserve the existing card as the
rollback image. Provision this same device with its existing matching
credentials and VID/PID—do not invent or replace them.

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

# Host UI/API: import once, then select the validated GameCube title.
# The API requires the existing administrator token and CSRF confirmation:
curl --cacert /path/to/host-ca.crt -u "admin:$WIIBRIDGE_ADMIN_TOKEN" \
  -H 'X-WiiBridge-CSRF: 1' -X POST \
  -d 'id=GAMEID&revision=0' \
  https://HOST:8445/api/v1/export/gamecube

# Pi: choose exactly one matching card mode, then attach.
sudo /usr/libexec/wiibridge-helper connect-gamecube-physical
# OR: sudo /usr/libexec/wiibridge-helper connect-gamecube-emulated
sudo /usr/libexec/wiibridge-helper attach
```

To return to Wii mode, detach/disconnect, POST `/api/v1/export/wii`, then:

```sh
sudo /usr/libexec/wiibridge-helper connect-wii
sudo /usr/libexec/wiibridge-helper attach
```

Proceed through `diagnostics/gamecube/hardware-validation.md`. Do not declare
production acceptance until all physical gates pass.
