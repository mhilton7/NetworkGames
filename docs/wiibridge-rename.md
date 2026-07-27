# WiiBridge naming migration

The project, Go module, container, firmware, services, runtime directories,
configuration variables, USB gadget name, logs, generated artifacts, and
documentation now use `WiiBridge`, `wiibridge`, or `WIIBRIDGE`.

This is a breaking deployment-name migration. Do not replace the live host and
Pi independently while a Wii session is active.

## Preserved identities

The existing certificates, private keys, administrator token, Wi-Fi profile,
USB VID/PID, `/etc/machine-id`, source-game paths, and TrueNAS datasets must be
reused unchanged. The USB serial retains its deployed `NG-<machine-id>` value
because it identifies the same physical bridge; changing it would force a new
USB identity and is not a cosmetic repository rename.

Historical backup paths, old immutable image references, and signed firmware
reports also retain their original names. They are evidence, not active
project identifiers.

## Host migration

1. Detach USB and disconnect NBD.
2. Snapshot the existing TrueNAS config, data, logs, backups, and certificate
   datasets.
3. Copy the current Compose environment values to the corresponding
   `WIIBRIDGE_*` variables in `deploy/truenas/.env.example`.
4. Keep every host dataset path and secret value unchanged.
5. Build/import the new `wiibridge-host` image and update the Compose service.
6. Confirm `/library` is read-only and `/data` contains the existing database
   and GameCube cache.

## Raspberry Pi migration

The new image uses:

```text
/etc/wiibridge
/run/wiibridge
/var/lib/wiibridge
/var/cache/wiibridge
/usr/libexec/wiibridge-helper
wiibridge-controller.service
```

Before replacing the SD card, preserve the existing device certificates,
administrator token, authorized VID/PID, Wi-Fi configuration, machine
identity, and bridge settings. Provision the rebuilt WiiBridge image with
those exact values. Do not generate replacement certificates or USB IDs merely
because paths changed.

After boot:

```sh
systemctl status wiibridge-controller.service
sudo /usr/libexec/wiibridge-helper test
sudo /usr/libexec/wiibridge-helper connect-wii
sudo /usr/libexec/wiibridge-helper attach
```

Then repeat the physical Wii and GameCube validation matrix before retiring
the rollback SD card.

## GitHub repository setting

The source files and workflow publish `ghcr.io/mhilton7/wiibridge-host`.
Renaming the GitHub repository itself is an external repository-setting action
and is intentionally not performed by this source edit.
