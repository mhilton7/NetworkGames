# Release gates

Status values are `PENDING`, `PASS`, `FAIL`, `BLOCKED_EXTERNAL`, and
`DEFERRED_HARDWARE_UNAVAILABLE`. Only executed checks may be marked `PASS`.

| Gate | Status | Evidence |
|---|---|---|
| Reproducible clean build | PENDING | Clean board builds passed; a second byte-for-byte full rebuild was not performed |
| Dependencies and revisions pinned | PASS | `versions.lock`, `go.sum` |
| Unit, security, and static analysis | PASS | `make test`, `make static` |
| FAT32 independent validation | PASS | `tests/fat32/fsck_test.go` |
| NBD mutual-TLS interoperability | PASS | `reports/libnbd-info.json`, kernel-client test |
| Mutation and failure injection | PASS | `tests/unit/scanner_test.go`, NBD protocol tests |
| Bounded cache | PASS | `tests/unit/cache_test.go` |
| 1,000-entry catalog | PASS | `tests/integration/catalog1000_test.go` |
| Server/container package | PASS | `dist/networkgames-host-0.1.0-rc.1.oci` |
| Hardened unprivileged container | PASS | `reports/container-test.json` |
| TrueNAS Compose validation | PASS | independent Docker Compose parser |
| TrueNAS live deployment | BLOCKED_EXTERNAL | Target not connected |
| Zero W image offline validation | PASS | `reports/firmware/zero-w-armhf/` |
| Pi 4 image offline validation | PASS | `reports/firmware/pi4-arm64/` |
| Pi 5 image offline validation | PASS | `reports/firmware/pi5-arm64/` |
| Pi 4/Pi 5 artifact independence | PASS | Distinct whole-image SHA-256 values |
| Artifact checksums/SBOM/provenance | PASS | Per-board files under `dist/` |
| Physical Raspberry Pi boot | DEFERRED_HARDWARE_UNAVAILABLE | |
| Physical USB gadget enumeration | DEFERRED_HARDWARE_UNAVAILABLE | |
| Separate Linux USB-host test | DEFERRED_HARDWARE_UNAVAILABLE | |
| Nintendo Wii detection | DEFERRED_HARDWARE_UNAVAILABLE | |
| USB Loader GX enumeration | DEFERRED_HARDWARE_UNAVAILABLE | |
| Legal game launch | DEFERRED_HARDWARE_UNAVAILABLE | |
| Gameplay soak | DEFERRED_HARDWARE_UNAVAILABLE | |
| Cable, power, reconnect, reboot | DEFERRED_HARDWARE_UNAVAILABLE | |
