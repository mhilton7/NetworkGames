# Worklog

## 2026-07-24 — Baseline

- Read the complete 1,678-line controlling prompt.
- Found an initial repository commit containing only project metadata; those
  tracked files were deleted in the incoming worktree and are being recreated
  to implement the requested system.
- Build host: Debian 13.6 amd64, kernel 6.12.96, approximately 167 GiB free.
- Available initially: Go 1.24, GCC 14, QEMU user emulators, bmaptool, OpenSSL,
  dosfstools, systemd validation, shellcheck, and xz.
- Docker/Podman, nbdkit command-line tools, and a connected TrueNAS target were
  not detected during the initial sandboxed inventory.
- Exact TrueNAS version, Apps backend, target datasets, ACLs, target CPU, and
  target port availability remain `TARGET_NOT_CONNECTED`; no deployment result
  is claimed.
- All physical tests are `DEFERRED_HARDWARE_UNAVAILABLE`.

## 2026-07-24 — Server, bridge, and release build

- Implemented scanner, immutable FAT32/MBR synthesis, direct source extents,
  source-mutation failure, SQLite snapshots, HTTPS control plane, and a bounded
  fixed-newstyle read-only NBD server.
- Passed Go unit/integration tests, 1,000-entry catalog synthesis, independent
  FAT32 fsck, real libnbd mutual-X.509 interoperability, and a Linux kernel NBD
  read-only mount. NBD uses the documented TLS 1.2 compatibility profile;
  HTTPS uses TLS 1.3.
- Built and tested the hardened non-root amd64 container with a read-only root,
  no capabilities, no privileged mode, no Docker socket/devices, persistent
  metadata, and an unchanged read-only WBFS source.
- Produced and independently parsed the TrueNAS Compose artifacts. No TrueNAS
  target is connected, so live TrueNAS facts and deployment remain external.
- Implemented Pi controller, bounded cache, board rejection, first-boot
  identity, fixed-action privileged helper, ConfigFS gadget lifecycle, systemd
  hardening, three pi-gen profiles, packaging, and offline validation.
- Discarded initial parallel pi-gen attempts after shared chroot `/dev/pts`
  teardown collisions. Later rejected attempts exposed missing stage-copy and
  pi-gen hook/export integration; no image from those attempts was accepted.
- Completed clean sequential pi-gen builds for Pi 5 ARM64, Pi 4 ARM64, and
  Zero W ARMHF. Sanitized machine identity and SSH host keys after pi-gen export,
  then validated the exact release images.
- All three images passed partition inspection, non-destructive FAT/ext4 checks,
  read-only inspection, architecture/board metadata, required boot and gadget
  configuration, systemd/application presence, QEMU application smoke tests,
  and embedded-payload/identity scans. Pi 4 and Pi 5 hashes are distinct.
- Generated each board's uncompressed/compressed checksums, bmap, package
  manifest, SPDX SBOM, provenance, offline report, and retained build log.
- Rebuilt the final amd64 OCI layout at
  `sha256:14dfeb29b5ac206c479be45a32bb1752ab56951c099053e757ad8f2915b50461`.
  Reran the hardened container, libnbd mutual-TLS, Linux kernel NBD read-only
  mount, Go, static, systemd, and Compose validation gates successfully.
- Final local status is `SOFTWARE-COMPLETE RELEASE CANDIDATE — HARDWARE
  UNVERIFIED`. Live TrueNAS deployment is still `BLOCKED_EXTERNAL` because no
  target is connected; full-build byte reproducibility remains `PENDING`.

## 2026-07-24 — GitHub publisher diagnostics

- Diagnosed a failed publication as an invalid active GitHub CLI token for
  `mhilton7`; no upload was claimed.
- Updated `scripts/publish_github.py` to retain command stderr in failures and
  derive an explicit `OWNER/REPO` selector from the configured Git remote.
- Passed Python bytecode compilation and verified that the current `origin`
  resolves to `mhilton7/NetworkGames`. A live upload was not attempted because
  GitHub CLI authentication must first be renewed.

## 2026-07-24 — GitHub Docker workflow

- Replaced two duplicate GitHub Docker workflow templates with one bounded
  workflow using `ubuntu-latest`, read-only repository permissions, concurrency
  cancellation, and a 20-minute timeout.
- Corrected the build to install the Go version declared by `go.mod`, create
  the required static amd64 server binary, and use
  `server/packaging/container/Dockerfile`.
- Parsed the workflow YAML, passed `git diff --check`, built the exact static
  server input, and successfully built the resulting local Docker image
  `networkgames-host:workflow-test`.
- The first corrected GitHub-hosted run passed all steps in 54 seconds. Updated
  Checkout and Setup Go to their Node.js 24 action releases after that run
  reported the older Node.js 20 action runtime as deprecated.

## 2026-07-24 — GHCR publishing and HTTPS port migration

- Added authenticated GHCR publishing for main-branch builds using the
  repository-scoped `GITHUB_TOKEN` and `packages: write`; no credential is
  recorded in the workflow.
- Migrated the server's internal HTTPS listener, container exposure, TrueNAS
  mapping, health check, examples, and operational documentation from TCP 8443
  to TCP 8445 because 8443 conflicts with an existing service.
- Passed server tests, static Compose policy validation, Docker Compose parsing,
  YAML parsing, formatting, and whitespace validation before publication.
## 2026-07-25 — First physical Zero W boot and setup-path repair

- The first physical Raspberry Pi Zero W boot of `0.1.0-rc.1` exposed a failed
  `networkgames-recover.service` and no configuration access point.
- Read-only card inspection proved first-boot identity generation completed,
  while the recovery helper rejected `detach` before provisioning, the image
  contained no AP profile/service or provisioning implementation, and the
  controller could not traverse its credential directory.
- Reworked cleanup actions so detach/disconnect remain fail-safe without bridge
  configuration; added a device-unique WPA2 NetworkManager setup/recovery AP,
  forced-recovery boot marker, authenticated and CSRF-protected HTTPS
  provisioning form, typed root provisioning helper, login rate limiting,
  corrected service permissions, and bounded persistent journals.
- Added controller tests and expanded shell, systemd, secret, and firmware
  static validation. `make test` and `make static` pass. The rebuilt static
  ARMv6 controller starts under QEMU.
- Repaired the connected card after an ext4 journal recovery. Device identity,
  admin token, and TLS credential hashes matched before and after. The card
  passes systemd verification, target-user controller startup, secure AP
  profile inspection, and unprovisioned recovery-detach execution.
- Physical reboot, AP association, HTTPS setup, Wi-Fi transition, NBD/TLS, USB
  gadget, Wii, and USB Loader GX tests have not yet been run on the repaired
  card. The prior release candidate is withdrawn pending retest and rebuild.
- The first repaired-card reboot confirmed `networkgames-recover.service`
  finished successfully and the controller started as intended. The setup AP
  still failed: persistent logs showed NetworkManager/wpa_supplicant attempting
  AP mode, followed by BCM43430 `key setting validation failed` and a
  supplicant timeout.
- Replaced NetworkManager AP mode with dedicated `hostapd` and `dnsmasq`
  services while retaining NetworkManager for client Wi-Fi. Installed the
  repository-matched Raspbian ARMHF hostapd package after verifying its SHA-256,
  masked the generic package service, preserved the device-specific AP
  passphrase, and installed only the hardened NetworkGames service graph.
  Offline hostapd parsing, dnsmasq syntax, systemd verification, package
  architecture, and device identity preservation pass. Physical association
  with the new AP remains pending.
- Physical hostapd retest then confirmed `AP-ENABLED`, iPhone association,
  `WPA: pairwise key handshake completed (RSN)`, and
  `EAPOL-4WAY-HS-COMPLETED`. The apparent password wait was DHCP: the hardened
  dnsmasq service repeatedly failed because its default lease path was
  read-only.
- Moved the lease database to
  `/run/networkgames-dnsmasq/dnsmasq.leases`, added a systemd-owned runtime
  directory, and ran dnsmasq as its unprivileged account with only
  `CAP_NET_ADMIN`, `CAP_NET_BIND_SERVICE`, and `CAP_NET_RAW`. Full automated
  tests, dnsmasq syntax validation, systemd verification, card identity
  preservation, and filesystem checks pass. Physical DHCP and HTTPS setup
  access remain pending.
- The DHCP-runtime retest associated successfully but assigned the iPhone a
  link-local `169.254.19.116` address. Persistent logs identified the remaining
  failure precisely: the unprivileged dnsmasq process could not traverse the
  protected `/etc/networkgames` credential directory to read its configuration.
- Moved the non-secret DHCP configuration to the root-owned, world-readable
  `/etc/networkgames-dnsmasq.conf` path while keeping all credentials protected.
  Repaired the live card without changing the AP passphrase, admin token,
  device certificate/key, machine identity, or setup file. `make test`,
  `make static`, systemd verification, and dnsmasq configuration parsing under
  the card's actual dnsmasq UID/GID pass. Physical DHCP and HTTPS setup access
  require one more reboot test.
- The next physical test obtained DHCP successfully and loaded the HTTPS login
  page, proving the hostapd, dnsmasq, routing, controller, and TLS setup path.
  Manual authentication with the original 64-character management credential
  remained unsuccessful.
- Changed new-device management passwords to 12 lowercase hexadecimal
  characters and changed the controller minimum accordingly. The live-card
  repair requires an explicit password-reset option, so routine repairs retain
  credentials. Added boundary tests, rebuilt the static ARMv6 controller, and
  reset only the live card's management password. The new value matches
  `NETWORKGAMES-SETUP.txt`; AP passphrase, TLS certificate/key, and machine
  identity hashes remain unchanged. Automated and offline checks pass; physical
  login with the short password remains pending.
- After the test card was reported corrupted, rebuilt the complete Zero W
  image from the current working tree rather than restoring the obsolete
  pre-repair artifact. The new build includes the ARMv6 controller, hostapd,
  dnsmasq runtime/configuration repairs, first-boot/recovery changes, and
  12-character management-password policy.
- The rebuilt image passed FAT/ext4 inspection, board and boot metadata,
  service/package presence, secret/identity absence, ARM architecture, QEMU
  smoke, checksums, compressed-image expansion, SBOM, and provenance checks.
  Flashed it to the confirmed removable `/dev/sdb` device.
- A whole image-region hash differed because bmap intentionally leaves
  unallocated sparse ranges untouched on reused media. Direct post-write
  comparison of all 163 allocated bmap ranges passed. The flashed card then
  passed FAT/ext4 checks, clean first-boot identity state, enabled-service
  inspection, exact controller-binary comparison, systemd verification, and
  dnsmasq parsing as its actual unprivileged UID/GID. Fresh first boot and
  management login remain pending.
- The fresh image completed its first physical Zero W boot and expanded the
  root filesystem. Persistent logs prove first-boot identity generation and
  recovery completed, the setup AP, hostapd, dnsmasq, and controller started,
  hostapd reached `AP-ENABLED`, and dnsmasq opened its intended DHCP range on
  `wlan0`. No NetworkGames failure appears in the boot journal.
- The abrupt card removal after that boot left only normal FAT/ext4 dirty
  state. Offline repair cleared the FAT dirty bit and recovered the ext4
  journal; repeat non-destructive filesystem checks are clean. The generated
  setup file matches the device SSID, setup Wi-Fi credential, 12-character
  management credential, and management URL. Client login remains pending.
- The fresh-image management login passed: the authenticated network-setup
  form accepted the 12-character management credential and reached submission.
  Client Wi-Fi provisioning then failed before profile creation because the
  controller unit's `ProtectSystem=strict` sandbox exposed only
  `/run/networkgames` as writable to the root provisioning helper.
- Added only the three persistent provisioning targets to the controller
  unit's `ReadWritePaths`: the NetworkManager connection directory,
  `/etc/networkgames`, and `/boot/firmware`. Filesystem ownership still blocks
  direct writes by the unprivileged controller. `make static`, `make test`,
  systemd verification, and whitespace checks pass.
- Installed the corrected unit on the physical card without resetting any
  credential or identity. No partial client profile or provisioned marker was
  present after the failed submission, and the setup file, management
  credential, AP credential, certificates, hostapd configuration, and machine
  identity were preserved. Client provisioning must now be retried.
- The repaired physical provisioning retest passed. The Pi saved the submitted
  2.4 GHz client profile and joined the configured home network after reboot.
  Local management now moves to HTTPS port 9443 on the Pi's DHCP address; NBD
  server/TLS provisioning and attachment remain pending.
- Revised provisioning so an existing client Wi-Fi profile is retained when
  all three Wi-Fi fields are blank; partial Wi-Fi updates remain rejected.
  Existing bridge settings likewise remain stored when their fields are blank.
- Added CSRF-protected browser buttons for server test/connect/disconnect and
  USB attach/detach, plus dashboard reporting for the automatically discovered
  USB Device Controller and its live link state.
- Added an opt-in boot service that tests mutual-TLS NBD, connects read-only,
  and attaches the USB gadget only when a complete bridge, an authorized
  VID/PID pair, and the auto-attach marker are present. Forced recovery disables
  it, failures clean up the connection, and systemd bounds retry attempts.
- Full Go tests, shell analysis, systemd verification, static checks, an ARMv6
  controller rebuild, and QEMU smoke pass. The feature is not yet installed on
  or physically tested with the live card, and no USB identity was invented.
- Installed the rebuilt ARMv6 controller, retained-WiFi provisioning helper,
  typed USB/NBD helper, and opt-in auto-attach unit on the physical Zero W
  card. The installed files exactly match the validated source and the complete
  systemd graph verifies against the mounted card.
- The card's client Wi-Fi profile, bridge material if present, setup file,
  management/AP credentials, TLS identity, hostapd configuration, and machine
  identity were byte-for-byte preserved. The auto-attach marker remains absent,
  so the new service cannot attach unexpectedly. Physical boot, UI, server,
  Linux USB-host, and Wii tests remain pending.

## 2026-07-25 — TrueNAS TLS health-check repair

- Confirmed the TrueNAS target at `192.168.0.175` is reachable and TCP ports
  8445 and 10809 accept connections. The deployed HTTPS health endpoint returns
  success, but strict certificate inspection proves that listener is still
  serving an older certificate that does not chain to the replacement CA.
- Identified the repeat startup defect: the Compose health check connects to
  `127.0.0.1`, while the newly issued server certificate originally contained
  only the external `192.168.0.175` IP subject alternative name.
- Updated `scripts/tls-provision.sh` to issue server certificates for the
  requested DNS/IP identity plus loopback `127.0.0.1`, with bounded IPv4
  validation. Hostname, IPv4, loopback-only, and invalid-IPv4 generation checks
  pass.
- Reissued only `server.crt` and `server.key` under the existing CA, preserving
  the CA and Pi client identity. The prior server pair is retained in a private
  rollback directory.
- Verified the replacement certificate chain, both IP identities, key match,
  and validity. Started the packaged server locally with the replacement bundle
  and passed its exact built-in health-check command against the loopback URL.
- TrueNAS SSH is refused and no authenticated management API/shell is
  connected. The replacement files are ready, but target-side installation and
  an application restart remain externally blocked.

## 2026-07-25 — Pi Zero NBD module preload repair

- The first browser server-connect action reached the privileged helper but
  failed with `modprobe: FATAL: Module nbd not found in directory
  /lib/modules/6.18.34+rpt-rpi-v6`.
- Verified the exact Zero W image contains the matching
  `kernel/drivers/block/nbd.ko.xz`, its dependency index entry, and
  `CONFIG_BLK_DEV_NBD=m`. This ruled out an omitted module or kernel/modules
  version mismatch.
- Reproduced the relevant systemd behavior: `ProtectKernelModules=yes` hides
  the module tree even from a root helper inherited through the protected
  controller service, so `modprobe` reports an existing module as not found.
- Added an early-boot `systemd-modules-load` entry and bounded
  `nbds_max=1` option. Removed module loading from the protected helper; it now
  requires the preloaded module and `/dev/nbd0`. Kernel-module protection
  remains enabled.
- Added explicit service ordering and strengthened offline image validation to
  require the NBD preload configuration, dependency entry, and module file for
  every included kernel. Unit, static, shell, systemd, and whitespace checks
  pass.
- The reconnected card had been removed without a clean shutdown. Cleared the
  FAT dirty state and recovered the ext4 journal/free-block accounting, then
  confirmed both filesystems were clean before installing the repair.
- Verified the card contains the exact matching v6 NBD module and dependency
  entry, installed the boot preload/options, corrected helper and service
  ordering, and verified the complete systemd graph.
- Hash comparisons confirmed the client Wi-Fi profile, bridge configuration,
  CA/client TLS material, device certificate/key, management credential,
  setup file, hostapd configuration, and machine identity were preserved.
  Cleanly unmounted the card and passed final non-destructive FAT/ext4 checks.
  The physical reboot then passed NBD preload and reached TLS negotiation.

## 2026-07-25 — Pi client TLS file diagnosis

- The physical connect action advanced beyond the repaired NBD module stage
  but `nbd-client` reported that the installed client certificate/key could not
  be loaded by GnuTLS.
- Verified the generated Pi pair has a valid client-auth certificate, matching
  private key, valid chain and dates, and parses successfully with OpenSSL and
  GnuTLS.
- Strictly verified the replacement TrueNAS HTTPS certificate is now live.
  Connected to the live NBD export with the generated Pi identity; mutual TLS
  passed, the export is read-only, and its virtual size is 1,284,936,704 bytes.
  This isolates the remaining failure to the certificate/key copy installed on
  the Pi.
- Added provisioning-time client-purpose chain verification so a future
  mismatched or unrelated client certificate cannot be saved alongside the
  server CA. Full unit, static, shell, systemd, and whitespace checks pass.
- Offline card inspection identified the immediate cause: `ca.crt`,
  `client.crt`, and `client.key` were all zero bytes after another unclean
  removal. Recovered the FAT/ext4 filesystems, then installed exact copies of
  the live-verified bundle with the intended root/networkgames ownership and
  mode 0640.
- Added an authenticated, CSRF-protected, typed safe-poweroff dashboard action.
  It fail-safely detaches USB and NBD, flushes storage, and uses an exact sudo
  rule to request systemd poweroff without blocking the web response.
- Rebuilt the static ARMv6 controller and passed unit, static, shell, systemd,
  QEMU smoke, and whitespace checks. Installed the controller, helper,
  provisioner, sudo rule, and TLS bundle on the physical card.
- Verified the card certificate chain, client purpose, key match, OpenSSL and
  GnuTLS parsing, exact bundle hashes, and service graph. Hash comparisons
  preserved Wi-Fi, bridge configuration, device identity, management
  credential, and setup/AP state. Final FAT/ext4 checks are clean; physical
  connection and safe-poweroff retests remain.
- The repaired physical retest loaded the client identity, completed mutual
  TLS negotiation, and received the expected 1,225 MB export size. This closes
  the client-file failure.
- `nbd-client` then failed its generic-netlink `NBD_CMD_CONNECT` while handing
  the negotiated socket to the kernel. Source inspection confirms the displayed
  error is emitted only when the netlink setup request is rejected. Persistent
  kernel logs are required to distinguish an occupied device, capability
  restriction, or Pi-kernel netlink incompatibility before applying another
  repair.

## 2026-07-25 — NBD idle timeout and USB VID/PID propagation repair

- Used the new dashboard poweroff action on the physical Pi and inspected its
  persistent journal after shutdown. Both card filesystems were clean, which
  physically validates the safe-poweroff path.
- Corrected the earlier NBD diagnosis: the first kernel setup succeeded.
  Journal and kernel evidence show `Connected /dev/nbd0`, the expected
  1,284,936,704-byte capacity, and partition `p1`.
- Correlated the Pi journal with the TrueNAS log. The deployed server closed a
  healthy idle NBD session after about 33 seconds because the 30-second
  negotiation deadline was retained during transmission. Subsequent attempts
  found `nbd0` already in use and emitted the misleading generic setup error.
- Changed the NBD server to clear the connection deadline once negotiation
  enters transmission. Added a regression test that remains idle beyond the
  negotiation deadline and then successfully performs a read.
- Hardened the Pi helper to wait for udev and read sector zero during its test
  action, and to reject an active connection while cleaning up stale NBD state
  before a real reconnect.
- Offline card inspection showed that a syntactically valid authorized USB
  VID/PID pair and the auto-attach marker were already present. The gadget
  failure was caused by the helper not exporting the sourced values to its
  child process. Added the missing exports without recording the pair's values.
- Passed `make test`, `make static`, `make server`, the hardened container
  lifecycle test, and whitespace validation. Built the replacement TrueNAS
  image archive
  `networkgames-host-0.1.0-rc.1-idlefix.1.docker.tar` with its checksum and
  target installation guide.
- Installed the updated helper and auto-attach unit on the physical card.
  Exact checks confirmed that Wi-Fi, TLS, bridge configuration, authorized
  pair, auto-attach choice, credentials, and machine identity were preserved.
  Final non-destructive FAT and ext4 checks passed.
- Live TrueNAS deployment remains externally blocked because no authenticated
  management shell or API is connected. The Pi must not be booted against the
  old server image for the final USB retest.
