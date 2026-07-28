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
| Server/container package | PASS | `dist/wiibridge-host-0.1.0-rc.1.oci` |
| Hardened unprivileged container | PASS | `reports/container-test.json` |
| TrueNAS Compose validation | PASS | independent Docker Compose parser |
| TrueNAS external/loopback TLS health-check compatibility | PASS | Reissued same-CA server certificate covers `192.0.2.10` and `127.0.0.1`; the packaged host's built-in health-check command passed against a local instance using the replacement bundle |
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
| Physical Zero W DHCP-runtime repair retest | FAIL | WPA2 association passed, but dnsmasq could not read its configuration inside the protected credential directory; iPhone received `192.0.2.10` |
| Physical Zero W DHCP-config-permission repair retest | PASS | iPhone received DHCP and loaded the controller HTTPS login page |
| Physical Zero W fresh-image first boot | PASS | Root expansion and identity generation completed; recovery, AP, hostapd, dnsmasq, and controller started; hostapd reached `AP-ENABLED` and dnsmasq opened the `10.77.0.20`-`10.77.0.100` DHCP range |
| Physical Zero W 12-character management-login retest | PASS | The authenticated network-setup form was reached and submitted with the fresh image's 12-character management credential |
| Physical Zero W client Wi-Fi provisioning retest | PASS | The repaired provisioning form saved the 2.4 GHz client profile, and the Pi joined the configured home network after reboot |
| Retained-WiFi save and USB automation source validation | PASS | Optional Wi-Fi update tests, CSRF-protected dashboard actions, bounded opt-in auto-attach unit, systemd verification, full automated suite, ARMv6 build, and QEMU smoke pass |
| Physical Zero W revised server/USB UI retest | FAIL | Server connect reached the typed helper, but its `modprobe nbd` inherited `ProtectKernelModules=yes` and saw the intentionally hidden `/lib/modules` tree |
| NBD boot-preload sandbox repair source validation | PASS | Actual v6 image contains matching `nbd.ko.xz`; boot preload/module options, protected helper checks, systemd ordering, unit/static tests, and strengthened firmware validation pass |
| Physical Zero W NBD boot-preload repair card installation | PASS | Matching v6 `nbd.ko.xz`, preload/options, helper, and service graph verified on the physical card; credentials, Wi-Fi, TLS, bridge state, and machine identity preserved; post-repair filesystems clean |
| Physical Zero W NBD boot-preload repair retest | PASS | Rebooted Pi advanced through NBD setup to TLS negotiation without the prior module error |
| Generated client identity against live TrueNAS NBD | PASS | Local known-good Pi bundle completes mutual TLS to `192.0.2.10:10809`, export `all` is read-only and reports 1,284,936,704 bytes |
| Physical Zero W installed client credential load | FAIL | Pi `nbd-client` reports that `/etc/wiibridge/client.crt` or `client.key` cannot be loaded; direct known-good file replacement is pending |
| Physical Zero W client TLS and safe-poweroff repair installation | PASS | All three prior Pi TLS files were zero bytes; exact live-verified bundle, stronger provisioning helper, ARMv6 controller, typed safe-poweroff action, and restricted sudo rule installed; unrelated state preserved and filesystems clean |
| Physical Zero W repaired client TLS retest | PASS | Pi loaded the repaired credential, completed TLS negotiation, and received the expected 1,225 MB export size |
| Physical Zero W first NBD kernel connection | PASS | Persistent journal shows `Connected /dev/nbd0`, the correct capacity, and partition `p1` |
| Physical Zero W NBD idle survival | PASS | After a clean reboot the dashboard remained NBD-connected beyond the prior failure interval while the Pi controller stayed reachable for a monitored 90 seconds |
| Server NBD idle-session repair | PASS | Transmission deadline cleared after negotiation; idle regression, full tests, build, and hardened container lifecycle pass |
| Pi stale-device/test/VID-PID export repair installation | PASS | Updated helper and auto-attach unit exactly installed; authorized pair, Wi-Fi, TLS, bridge state, and identity preserved; filesystems clean |
| TrueNAS idlefix live deployment | PASS | GHCR image is deployed; strict HTTPS, mutual-TLS NBD, exact 1,284,936,704-byte size, and a read after 35 idle seconds pass |
| Physical Zero W repaired USB attach/Windows read | FAIL | Windows enumerated the gadget and the Pi reported USB link `configured`, but Windows requested formatting because the FAT32 BPB had zero legacy geometry fields |
| Windows FAT32 BPB geometry repair source validation | PASS | BPB now records 63 sectors/track, 255 heads, and 2,048 hidden sectors; `fsck.vfat`, independent `mtools` directory read, unit, static, and whitespace checks pass |
| TrueNAS Windows FAT32 replacement deployment | PASS | Live idlefix.2 export reports 63/255/2,048 BPB geometry and independent `mtools` reads both the root and `/wbfs` directories |
| Physical Windows idlefix.2 corrected FAT32 mount | FAIL | Windows still requested formatting; card journal shows continuous NBD/USB service with no host-read errors, while the fixed, read-only MBR disk had signature `0x00000000` |
| Windows fixed-disk identity repair source validation | PASS | Deterministic nonzero per-catalog MBR signature and matching FAT volume ID; unit, FAT, static, and whitespace checks pass |
| Pi partition-readiness repair installation | PASS | Replaced global `udevadm settle` with bounded `nbd0p1` readiness and direct disk/partition reads; exact helper installed, state preserved, filesystems clean |
| TrueNAS idlefix.3 live deployment | PASS | Nonzero MBR signature, matching FAT volume ID, 63/255/2,048 geometry, `mtools`, `fsck.fat`, and kernel read-only FAT mount all pass against the live export |
| Physical Windows idlefix.3 partition recognition | PASS | Operator reports that Windows detects and visually presents the partition; an explicit File Explorer `/wbfs` read remains to be captured |
| Physical Pi 4/Pi 5 boot | DEFERRED_HARDWARE_UNAVAILABLE | |
| Physical USB gadget enumeration | DEFERRED_HARDWARE_UNAVAILABLE | |
| Separate Linux USB-host test | DEFERRED_HARDWARE_UNAVAILABLE | |
| Nintendo Wii detection | PASS | Wii and USB Loader GX mount the virtual storage and display its game file |
| USB Loader GX enumeration | PASS | Loader reaches the catalog and displays the game file; post-failure Pi status remains NBD-connected, USB-attached, and configured |
| Original single WBFS payload and FAT extent validation | PASS | The earlier one-file live export contained one contiguous WBFS extent; Wiimms ISO Tool 3.01a verified both update and data partitions as OK |
| Current four-file catalog integrity | FAIL | The live export is now 3,619,423,232 bytes with four files and no FAT free space; three files pass Wiimms ISO Tool verification and one fails an H0 data-partition integrity check |
| Wii cIOS no-stall compatibility retest | FAIL | With endpoint stalls disabled, USB Loader GX still returned to the Wii System Menu on game boot; the experiment has been reverted |
| First targeted cIOS-handoff trace capture | FAIL | Wii reboot reproduced, but repeated tracefs function-list scans consumed 78 seconds of Pi Zero CPU and the capture file remained empty |
| Corrected targeted cIOS-handoff trace capture | PASS | 1,749 trace lines capture the real launch; NBD requests complete, no real-session block error occurs, and five host writes take the mass-storage function's immediate write-protected path |
| Physical strict read-only LUN cIOS compatibility | FAIL | The final rejected host write preceded the return-to-menu reset by about 596 ms; the controlled writable-overlay comparison rejected SCSI-level write protection alone, while DOS read-only file attributes remained unchanged |
| Physical volatile COW write-compatibility retest | FAIL | The overlay accepted all five host writes and every one of 117 NBD requests completed, but game launch still returned to the System Menu; the experiment was reverted |
| Attempted legal-game payload integrity | PASS | The operator identified 10 Minute Solution; its catalog entry is one of the three current files that passes full Wiimms ISO Tool verification |
| Wii d2x slot/base inventory | PASS | SysCheck HDE reports d2x-v11-beta3 at 248[38], 249[56], 250[57], and 251[58] |
| Controlled game IOS250/base57 configuration | PASS | Only the attempted game's USB Loader GX record was changed from inherited 249/base56 to explicit 250/base57; system IOS installations were not modified |
| Wii SD post-configuration integrity | PASS | Reconciled differing redundant FAT copies, then `fsck.vfat -n` passed cleanly and the IOS250 game setting and SysCheck report were read back from a read-only remount |
| Physical IOS250/base57 game launch | FAIL | The verified payload returned to the Wii System Menu exactly as it did through IOS249/base56 |
| Physical IOS251/base58 game launch | FAIL | The verified payload again returned to the Wii System Menu; all installed d2x USB bases now fail identically |
| Attempted-game boot-file extraction | PASS | Fresh live-export copy has contiguous FAT allocation, valid IMET/U8 banner archive with banner/icon/sound entries, valid main DOL, IOS53 TMD, and both partitions pass verification |
| Second verified high-LBA title banner/launch | FAIL | American Mensa Academy also has a blank banner and returns to the System Menu after a tiled Wii-logo framebuffer flash |
| Verified low-LBA title banner/launch | FAIL | The Squeakquel below 2 GiB also has a blank banner and returns to the System Menu, eliminating disk offset as the differentiator |
| USB Loader GX WBFS open-mode diagnosis | PASS | Official r1283 source enumerates headers read-only but opens WBFS files with `O_RDWR` for banner and boot access; all live WBFS FAT entries carry DOS read-only attribute `0x01` |
| FAT WBFS attribute compatibility repair | PASS | Builder emits archive `0x20` without read-only `0x01`; byte-level and independent `mattrib` regressions, full tests/static checks, FAT validation, and hardened container lifecycle pass while NBD/LUN remain read-only |
| GHCR WBFS attribute replacement image | PASS | Commit `74dfb5b`; `ghcr.io/OWNER/wiibridge-host:0.1.0-rc.1-wbfsattrfix.1@sha256:37d94d0c3f11ae8c33f96490fe9c0902b6b417907afd56a14d8dd370f4b2fe80` published and anonymously resolved |
| TrueNAS WBFS attribute replacement deployment | PASS | Strict HTTPS health passes; the 3,619,423,232-byte mutual-TLS NBD export remains read-only, and independent live `mattrib` inspection reports archive `A` without read-only `R` on all four WBFS entries |
| Physical banner extraction after attribute repair | PASS | USB Loader GX now displays the 10 Minute Solution banner through the repaired live export |
| Legal game launch | PASS | 10 Minute Solution successfully loads on the Wii through the Zero W bridge and repaired TrueNAS export |
| Gameplay soak | PENDING | Sustained gameplay plus a later power/reconnect cycle remain to be exercised |
| Cable, power, reconnect, reboot | PASS | Repeated reboot, NBD reconnect, USB reset, cIOS handoff, and trace captures completed without Pi transport failure |
| GameCube schema-2 no-copy generation | PASS | Complete FAT32 metadata and sorted source extents are generated without `library.img` or payload-copy helper calls |
| GameCube no-copy physical storage | PASS | 6,291,456 source bytes produced 2,252,800 physically allocated metadata bytes for an 8,589,934,592-byte apparent disk; retained-generation regression passes |
| GameCube source integrity and read-through | PASS | ISO/GCM/CISO/two-disc/FST reads match original sources; size, hash, symlink, escape, overlap, duplicate-path, and changed-FST checks pass |
| GameCube physical memory-card mode | PASS | Backend and NBD profile are read-only and every write is rejected |
| GameCube emulated memory-card overlay | DEFERRED | Startup is explicitly rejected; no copied-image fallback is present |
| Wii synthetic disk regression | PASS | FAT32, integration, unit, and race suites pass after replacing resident FAT storage with byte-compatible on-demand synthesis |
| Physical GameCube Wii/Nintendont acceptance | DEFERRED_HARDWARE_UNAVAILABLE | Host-side FAT32 and storage behavior pass; no claim of physical compatibility is made |
| TrueNAS multi-terabyte Wii FAT memory | PASS | 8 GiB fixture retains 5 chain descriptors and 10,752 non-FAT metadata bytes instead of at least 16,778,240 resident raw FAT bytes; representation scales with files rather than clusters |
| TrueNAS container memory bound | PASS | Compose parser accepts a 384 MiB Go target inside a 512 MiB hard limit |
| GameCube 10,000-extent lookup | PASS | Binary search benchmark: 10.51 ns/op, approximately 95.1 million lookups/s, 0 B/op, 0 allocs/op |
| GameCube source-handle bound | PASS | Repeated reads reuse one handle; 40-source test and 10,000-read test never exceed the 32-handle LRU limit; close releases all handles |
| GameCube read coalescing | PASS | A 1 MiB request inside one ISO produces one source ReadAt; a 128 KiB request crossing two source extents produces exactly two |
| GameCube host performance matrix | PASS | Sequential, random, 32-source switching, 5,000-file FST, boundary, and concurrent benchmarks recorded in `reports/gamecube-no-copy-performance.json` |
| End-to-end physical GameCube performance | DEFERRED_HARDWARE_UNAVAILABLE | Host benchmarks and mutual-TLS protocol tests do not prove Pi Zero W/Wii/Nintendont gameplay throughput |
| Bounded Wii Host startup | PASS | 512 GiB fixture with 1,048,577 FAT sectors per copy builds from 131 compact chains in under one millisecond without walking the apparent FAT |
| Responsive UI during source scan | PASS | HTTPS and a phase-specific startup page are available before library walking; NBD remains unavailable until the validated Wii export is complete |
| Diagnostic startup and health logging | PASS | Phase transitions, 30-second heartbeats, scan counts, elapsed time, and exact CA/TLS/HTTP health errors are logged |
| Legacy loopback health certificate | PASS | Local check validates the trusted server-auth chain without requiring a 127.0.0.1 SAN; external identity verification is unchanged |
| HTTPS liveness before persistent/library work | PASS | Startup handler and listener are installed before LibraryManager, browser auth, SQLite, Wii scan, or GameCube scan; delayed-phase and failure regressions pass |
| Separate Host readiness | PASS | `/healthz` remains live during startup while `/readyz` stays 503 until the Wii backend/export manager is safe |
| GameCube startup validation bound | PASS | Fast validation checks compact generation data and source identities without payload hashing; deep hashing is background, cancellable, receipt-backed, and does not block Wii |
| Immutable binary and OCI source identity | PASS | Host `version` reports commit/build/dirty/Go/target and OCI builders emit revision/version/created/source labels with CI equality checks |
| Corrected TrueNAS startup deployment | PENDING | Exact digest, binary revision, OCI label, prompt 8445 liveness, and two consecutive restart timings require the operator's TrueNAS runtime |
