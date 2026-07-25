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
| TrueNAS external/loopback TLS health-check compatibility | PASS | Reissued same-CA server certificate covers `192.168.0.175` and `127.0.0.1`; the packaged host's built-in health-check command passed against a local instance using the replacement bundle |
| TrueNAS live deployment | PASS | Strict HTTPS verification accepts the replacement CA/IP certificate, and the live NBD export accepts the generated client identity, reports read-only, and returns a 1,284,936,704-byte virtual disk |
| Zero W original image offline validation | PASS | `reports/firmware/zero-w-armhf/`; offline checks did not detect the later physical setup failures |
| Zero W repaired source/card offline validation | PASS | `make test`, `make static`, ARMv6 QEMU smoke, live-card service-identity/controller/detach checks |
| Zero W repaired image rebuild | PASS | Clean current-source pi-gen build, offline firmware validation, hashes, bmap, SBOM, and provenance completed 2026-07-25 UTC |
| Zero W repaired image physical flash/readback | PASS | All 163 allocated bmap ranges match the source image; flashed FAT/ext4, services, clean identity, controller binary, and dnsmasq-user checks pass |
| Pi 4 image offline validation | PASS | `reports/firmware/pi4-arm64/` |
| Pi 5 image offline validation | PASS | `reports/firmware/pi5-arm64/` |
| Pi 4/Pi 5 artifact independence | PASS | Distinct whole-image SHA-256 values |
| Artifact checksums/SBOM/provenance | PASS | Per-board files under `dist/` |
| Physical Zero W initial boot | FAIL | Original rc.1 boot exposed failed recovery service and missing setup AP |
| Physical Zero W first repaired-card retest | FAIL | Recovery/controller passed; NetworkManager/wpa_supplicant AP failed to install WPA key on BCM43430 |
| Physical Zero W hostapd-card retest | FAIL | WPA2 handshake passed; dnsmasq lease file was blocked by service filesystem hardening |
| Physical Zero W DHCP-runtime repair retest | FAIL | WPA2 association passed, but dnsmasq could not read its configuration inside the protected credential directory; iPhone received `169.254.19.116` |
| Physical Zero W DHCP-config-permission repair retest | PASS | iPhone received DHCP and loaded the controller HTTPS login page |
| Physical Zero W fresh-image first boot | PASS | Root expansion and identity generation completed; recovery, AP, hostapd, dnsmasq, and controller started; hostapd reached `AP-ENABLED` and dnsmasq opened the `10.77.0.20`-`10.77.0.100` DHCP range |
| Physical Zero W 12-character management-login retest | PASS | The authenticated network-setup form was reached and submitted with the fresh image's 12-character management credential |
| Physical Zero W client Wi-Fi provisioning retest | PASS | The repaired provisioning form saved the 2.4 GHz client profile, and the Pi joined the configured home network after reboot |
| Retained-WiFi save and USB automation source validation | PASS | Optional Wi-Fi update tests, CSRF-protected dashboard actions, bounded opt-in auto-attach unit, systemd verification, full automated suite, ARMv6 build, and QEMU smoke pass |
| Physical Zero W revised server/USB UI retest | FAIL | Server connect reached the typed helper, but its `modprobe nbd` inherited `ProtectKernelModules=yes` and saw the intentionally hidden `/lib/modules` tree |
| NBD boot-preload sandbox repair source validation | PASS | Actual v6 image contains matching `nbd.ko.xz`; boot preload/module options, protected helper checks, systemd ordering, unit/static tests, and strengthened firmware validation pass |
| Physical Zero W NBD boot-preload repair card installation | PASS | Matching v6 `nbd.ko.xz`, preload/options, helper, and service graph verified on the physical card; credentials, Wi-Fi, TLS, bridge state, and machine identity preserved; post-repair filesystems clean |
| Physical Zero W NBD boot-preload repair retest | PASS | Rebooted Pi advanced through NBD setup to TLS negotiation without the prior module error |
| Generated client identity against live TrueNAS NBD | PASS | Local known-good Pi bundle completes mutual TLS to `192.168.0.175:10809`, export `all` is read-only and reports 1,284,936,704 bytes |
| Physical Zero W installed client credential load | FAIL | Pi `nbd-client` reports that `/etc/networkgames/client.crt` or `client.key` cannot be loaded; direct known-good file replacement is pending |
| Physical Zero W client TLS and safe-poweroff repair installation | PASS | All three prior Pi TLS files were zero bytes; exact live-verified bundle, stronger provisioning helper, ARMv6 controller, typed safe-poweroff action, and restricted sudo rule installed; unrelated state preserved and filesystems clean |
| Physical Zero W repaired client TLS retest | PASS | Pi loaded the repaired credential, completed TLS negotiation, and received the expected 1,225 MB export size |
| Physical Zero W first NBD kernel connection | PASS | Persistent journal shows `Connected /dev/nbd0`, the correct capacity, and partition `p1` |
| Physical Zero W NBD idle survival | FAIL | The old TrueNAS image closed the healthy idle transmission after about 33 seconds; later retries found `nbd0` still in use |
| Server NBD idle-session repair | PASS | Transmission deadline cleared after negotiation; idle regression, full tests, build, and hardened container lifecycle pass |
| Pi stale-device/test/VID-PID export repair installation | PASS | Updated helper and auto-attach unit exactly installed; authorized pair, Wi-Fi, TLS, bridge state, and identity preserved; filesystems clean |
| TrueNAS idlefix live deployment | BLOCKED_EXTERNAL | Replacement Docker archive and checksum are ready; loading the image and redeploying the target app require TrueNAS management access |
| Physical Zero W repaired USB attach/Windows read | PENDING | Blocked on deployment of the repaired TrueNAS server image |
| Physical Pi 4/Pi 5 boot | DEFERRED_HARDWARE_UNAVAILABLE | |
| Physical USB gadget enumeration | DEFERRED_HARDWARE_UNAVAILABLE | |
| Separate Linux USB-host test | DEFERRED_HARDWARE_UNAVAILABLE | |
| Nintendo Wii detection | DEFERRED_HARDWARE_UNAVAILABLE | |
| USB Loader GX enumeration | DEFERRED_HARDWARE_UNAVAILABLE | |
| Legal game launch | DEFERRED_HARDWARE_UNAVAILABLE | |
| Gameplay soak | DEFERRED_HARDWARE_UNAVAILABLE | |
| Cable, power, reconnect, reboot | DEFERRED_HARDWARE_UNAVAILABLE | |
