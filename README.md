# NetworkGames HostBridge

NetworkGames HostBridge exposes existing WBFS files in place as an immutable,
synthesized MBR/FAT32 disk over mutual-TLS NBD. A board-specific Raspberry Pi OS
bridge connects that export to a read-only Linux ConfigFS mass-storage LUN.

This repository is a software release candidate. Physical Raspberry Pi, USB,
Wii, and USB Loader GX tests are unavailable and are recorded as
`DEFERRED_HARDWARE_UNAVAILABLE`; no hardware compatibility claim is made.

The host runs only as a hardened TrueNAS Community Edition Custom App. The
library bind mount is mandatory read-only. Persistent app datasets contain only
configuration, compact snapshots/extents, certificates, logs, and backups—not a
payload mirror or payload-bearing raw disk image.

Build and test:

```sh
make test static server oci
make firmware-all
make validate-firmware
```

See [TrueNAS deployment](deploy/truenas/README.md),
[security model](docs/security.md), [Pi first run](docs/pi-first-run.md), and
[future hardware acceptance](docs/hardware-acceptance-plan.md).
