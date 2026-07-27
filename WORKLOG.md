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
  resolves to `mhilton7/WiiBridge`. A live upload was not attempted because
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
  `wiibridge-host:workflow-test`.
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
  `wiibridge-recover.service` and no configuration access point.
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
- The first repaired-card reboot confirmed `wiibridge-recover.service`
  finished successfully and the controller started as intended. The setup AP
  still failed: persistent logs showed NetworkManager/wpa_supplicant attempting
  AP mode, followed by BCM43430 `key setting validation failed` and a
  supplicant timeout.
- Replaced NetworkManager AP mode with dedicated `hostapd` and `dnsmasq`
  services while retaining NetworkManager for client Wi-Fi. Installed the
  repository-matched Raspbian ARMHF hostapd package after verifying its SHA-256,
  masked the generic package service, preserved the device-specific AP
  passphrase, and installed only the hardened WiiBridge service graph.
  Offline hostapd parsing, dnsmasq syntax, systemd verification, package
  architecture, and device identity preservation pass. Physical association
  with the new AP remains pending.
- Physical hostapd retest then confirmed `AP-ENABLED`, iPhone association,
  `WPA: pairwise key handshake completed (RSN)`, and
  `EAPOL-4WAY-HS-COMPLETED`. The apparent password wait was DHCP: the hardened
  dnsmasq service repeatedly failed because its default lease path was
  read-only.
- Moved the lease database to
  `/run/wiibridge-dnsmasq/dnsmasq.leases`, added a systemd-owned runtime
  directory, and ran dnsmasq as its unprivileged account with only
  `CAP_NET_ADMIN`, `CAP_NET_BIND_SERVICE`, and `CAP_NET_RAW`. Full automated
  tests, dnsmasq syntax validation, systemd verification, card identity
  preservation, and filesystem checks pass. Physical DHCP and HTTPS setup
  access remain pending.
- The DHCP-runtime retest associated successfully but assigned the iPhone a
  link-local `169.254.19.116` address. Persistent logs identified the remaining
  failure precisely: the unprivileged dnsmasq process could not traverse the
  protected `/etc/wiibridge` credential directory to read its configuration.
- Moved the non-secret DHCP configuration to the root-owned, world-readable
  `/etc/wiibridge-dnsmasq.conf` path while keeping all credentials protected.
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
  `WIIBRIDGE-SETUP.txt`; AP passphrase, TLS certificate/key, and machine
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
  `wlan0`. No WiiBridge failure appears in the boot journal.
- The abrupt card removal after that boot left only normal FAT/ext4 dirty
  state. Offline repair cleared the FAT dirty bit and recovered the ext4
  journal; repeat non-destructive filesystem checks are clean. The generated
  setup file matches the device SSID, setup Wi-Fi credential, 12-character
  management credential, and management URL. Client login remains pending.
- The fresh-image management login passed: the authenticated network-setup
  form accepted the 12-character management credential and reached submission.
  Client Wi-Fi provisioning then failed before profile creation because the
  controller unit's `ProtectSystem=strict` sandbox exposed only
  `/run/wiibridge` as writable to the root provisioning helper.
- Added only the three persistent provisioning targets to the controller
  unit's `ReadWritePaths`: the NetworkManager connection directory,
  `/etc/wiibridge`, and `/boot/firmware`. Filesystem ownership still blocks
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
  the live-verified bundle with the intended root/wiibridge ownership and
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
  `wiibridge-host-0.1.0-rc.1-idlefix.1.docker.tar` with its checksum and
  target installation guide.
- Installed the updated helper and auto-attach unit on the physical card.
  Exact checks confirmed that Wi-Fi, TLS, bridge configuration, authorized
  pair, auto-attach choice, credentials, and machine identity were preserved.
  Final non-destructive FAT and ext4 checks passed.
- Live TrueNAS deployment remains externally blocked because no authenticated
  management shell or API is connected. The Pi must not be booted against the
  old server image for the final USB retest.

## 2026-07-25 — GHCR publication and live idlefix verification

- Published the repaired amd64 container as
  `ghcr.io/mhilton7/wiibridge-host:0.1.0-rc.1-idlefix.1` from source commit
  `1fd587a2cf4e106575e8f13ddc2ab2ed34389fda`. Verified the remote manifest
  digest and anonymous pull access.
- The operator deployed the new image on TrueNAS. Strict HTTPS validation with
  the replacement CA passed against `192.168.0.175:8445`.
- Connected to the live export on port 10809 with the known-good client
  identity through a read-only libnbd FUSE client. The exported size was
  exactly 1,284,936,704 bytes.
- Held the live NBD transmission idle for 35 seconds, beyond the former
  30-second deadline, and then successfully read sector zero. This confirms
  the deployed server no longer drops a healthy idle block-device session.
- The remaining validation is the controlled physical Zero W boot, an
  additional idle interval through its kernel NBD client, and Windows USB
  gadget enumeration/read.

## 2026-07-25 — Windows FAT32 geometry diagnosis and repair

- After rebooting to clear prior stale NBD state, the physical Zero W remained
  NBD-connected beyond the former failure interval while its HTTPS controller
  stayed reachable throughout a monitored 90-second period.
- Windows enumerated the USB mass-storage gadget. The dashboard reported USB
  attached and link state `configured`, closing the VID/PID propagation and
  gadget-enumeration failures.
- Windows requested formatting rather than mounting the read-only export.
  A separate read-only connection to the same live export confirmed a valid
  DOS partition table with one type-0x0c partition at sector 2,048, then
  reproduced the rejection with `mtools`: the FAT32 BPB advertised zero heads
  and zero sectors-per-track.
- Updated the synthesized FAT32 BPB to record conventional LBA-assisted
  geometry of 63 sectors/track and 255 heads, plus the partition start as
  2,048 hidden sectors.
- Added exact unit assertions and an independent `mtools` directory-read
  regression alongside the existing `fsck.vfat` test. Full unit, static, and
  whitespace validation passes.
- The hardened container lifecycle passed with the repaired builder. Published
  the cumulative replacement as
  `ghcr.io/mhilton7/wiibridge-host:0.1.0-rc.1-idlefix.2`, verified its
  remote manifest digest and anonymous pull access.
- After target redeployment, strict HTTPS and mutual-TLS NBD connection passed.
  The live export reports the corrected 63 sectors/track, 255 heads, and 2,048
  hidden sectors. Independent read-only `mtools` access to both the filesystem
  root and `/wbfs` directory passes. Physical Windows remount is the remaining
  step.

## 2026-07-25 — Windows fixed-disk identity and Pi readiness repair

- The idlefix.2 Windows remount still requested formatting. A live read-only
  loop-device probe identified the export as FAT32, `fsck.fat` passed, the
  Linux kernel mounted it read-only, and `/wbfs` was readable.
- Inspected the safely shut-down Pi's persistent journal from the exact
  attempt. The final NBD connection remained active until the requested
  poweroff, USB reached high-speed configured state, and no host-read, gadget,
  or NBD errors occurred while Windows inspected the disk.
- The journal also showed the first auto-attach test unnecessarily failing
  because global `udevadm settle --timeout=5` timed out, then disconnecting
  while partition reads were pending. A bounded retry later succeeded.
- Isolated the remaining Windows-specific metadata defect: the gadget presents
  a fixed, read-only MBR disk, while the MBR signature was zero. Added a
  deterministic nonzero per-catalog MBR signature and matching FAT volume ID
  so Windows receives a stable unique disk/volume identity without needing to
  write one.
- Replaced global udev settling with a bounded wait for `/dev/nbd0p1` and
  direct reads from both the whole disk and partition before disconnecting the
  test session.
- Full unit, FAT, static, shell, and whitespace validation passes. Installed
  the updated helper on the physical card; Wi-Fi, TLS, bridge configuration,
  authorized VID/PID, auto-attach state, credentials, and machine identity
  were preserved, and both card filesystems remain clean.
- The hardened container lifecycle passed. Published cumulative idlefix.3 from
  commit `c2a529479fb3d93d46e25e6da157dea1e001994f`, verified the remote
  manifest digest and anonymous pull access. TrueNAS deployment and physical
  Windows retest remain pending.
- After target deployment, strict HTTPS and mutual-TLS NBD passed. The live
  export has a nonzero deterministic MBR signature, a matching FAT volume ID,
  the corrected geometry, and passes independent `mtools`, `fsck.fat`, and
  Linux kernel read-only mount checks. Physical Windows remount is next.

## 2026-07-25 — Windows recognition and first Wii attempt

- The operator reported that Windows now detects and visually presents the
  idlefix.3 partition. This closes the fixed-disk identity defect at the
  partition-recognition layer; a separate explicit File Explorer `/wbfs` read
  has not yet been recorded.
- On the first available Wii test, USB Loader GX crashed during startup when
  the Zero W bridge was connected. No game launch was attempted, and no exact
  loader revision, cIOS inventory, crash screen, or Pi journal from the
  attempt has been captured yet.
- Inspection of the current official USB Loader GX r1283 source confirms that
  USB port 0 is the default and that USB initialization occurs early in its
  startup. Its release notes include partition-detection and size-calculation
  repairs, and its official setup requires current d2x cIOS.
- The next controlled test is to leave the Wii at its system menu for 90
  seconds so the Pi can finish booting, connect NBD, and attach the gadget
  before launching the loader. Loader revision, crash mode, selected USB port,
  and whether the app/configuration reside on SD must be captured before any
  speculative disk-format or gadget change.
- The follow-up result advances beyond startup: USB Loader GX mounts the
  virtual storage and displays the game file, but crashes when asked to boot
  it. A post-failure controller probe still reports NBD connected, USB
  attached, and gadget state `configured`. The filesystem/catalog path is
  therefore physically proven through the Wii, while cIOS handoff, transient
  USB reset behavior, and full WBFS payload validity remain under diagnosis.
- Persistent journal inspection from two launch attempts shows the Wii
  resetting and re-enumerating the DWC2 gadget at the cIOS handoff. Both
  sequences reached high speed and cleared the bulk endpoint halts, while DWC2
  warned that it could not clear a halt on control endpoint zero. NBD remained
  connected and no block-read, TLS, or network error occurred during either
  attempt. The operator reports a clean reboot to the Wii System Menu rather
  than a DSI exception.
- Mounted the live export read-only and confirmed that it contains one WBFS
  file in a single contiguous FAT cluster extent. Wiimms ISO Tool 3.01a
  verified both the update and data partitions as `OK`; source payload
  corruption and FAT fragmentation are not supported by the evidence.
- Added the Linux mass-storage function's supported no-stall setting before
  binding the gadget. This preserves the read-only LUN and immutable NBD
  backing while avoiding bulk endpoint stalls during the legacy d2x reset
  sequence. Static, shell, and whitespace validation pass, and the exact
  helper is installed on the physical card for the next Wii boot test.
- The physical no-stall retest still returned cleanly to the Wii System Menu
  when game boot was requested. This rejects endpoint-stall suppression as the
  repair. Removed the override and its temporary validator assertions from
  source, and restored the normal gadget helper on the card.
- Installed and enabled temporary, narrowly filtered boot-time tracing for the
  mass-storage SCSI path, DWC2 halt handling, NBD request lifecycle, and block
  errors. Trace output persists in
  `/var/log/wiibridge-usb-trace.log`; Wi-Fi, TLS, bridge configuration,
  automatic attach selection, and machine identity remain unchanged.
- Verified exact installed files, service enablement, retained state, shell
  syntax, systemd unit validity, the complete unit/static suite, and whitespace.
  Non-destructive FAT and ext4 checks both pass, and the reader was powered off
  before removal.
- The next physical launch reproduced the clean return to the Wii System Menu.
  The normal journal again shows the Wii resetting and re-enumerating the
  gadget, with the live NBD session intact and no network, block-read, or TLS
  failure. The intended trace file was zero bytes even though its service
  stayed active and consumed 78 seconds of CPU.
- Isolated the instrumentation defect to reopening the expensive tracefs
  function inventory for each requested symbol on the ARMv6 Pi Zero. Reworked
  initialization to scan that inventory once, write progress directly to the
  protected capture file and persistent journal, reduce the trace buffer, and
  signal systemd readiness only after tracing is active. USB auto-attach now
  waits for that readiness signal. Installed the corrected helper/unit exactly
  with retained Wi-Fi, TLS, bridge, auto-attach, and machine state verified.
  FAT and ext4 checks pass after installation, and the reader was powered off
  before removal.
- The corrected physical capture succeeded with 1,749 trace lines. Every NBD
  request in the real gadget session completed, all five block-error events
  belong to the intentional pre-attach test disconnect, and the Wii/cIOS path
  continued issuing successful reads after USB reset and re-enumeration.
- Captured five write commands from the host. Each returned immediately
  without entering the mass-storage function's data-receive path, matching the
  kernel's explicit `ro` branch that reports `WRITE PROTECTED`. The last
  rejection at monotonic time 209.666114 was followed by gadget disable/reset
  at 210.262302, about 596 ms later. Because earlier rejected writes were
  survived, this is treated as a controlled compatibility hypothesis rather
  than a proven root cause.
- Added a volatile write-compatibility view using a 32 MiB, tmpfs-backed,
  nonpersistent device-mapper snapshot. The NBD origin must remain
  kernel-enforced read-only; host writes are redirected to RAM and disappear
  at detach or reboot. A local loop-device test accepted and read back overlay
  writes while the read-only origin remained byte-identical.
- Added exact overlay setup/cleanup, stale-loop recovery, failure cleanup,
  device size/read-only assertions, device-mapper module preload, explicit
  `dmsetup` packaging, image installation, repair-card installation, and
  strengthened static/offline checks. Full unit/static/whitespace validation
  passes. The physical card has exact updated helper, gadget, overlay, trace
  unit, and module-preload files; retained state is verified. The successful
  strict-read-only capture was preserved separately for comparison. FAT and
  ext4 checks pass after installation, and the reader was powered off before
  removal.
- The physical comparison capture contains 3,560 lines. The overlay accepted
  all five writes and entered the normal data-receive path, 390 reads
  completed, and all 117 NBD requests had matching header and payload replies.
  The only five block errors belong to the intentional pre-attach disconnect
  test. The final accepted write preceded the gadget reset by about 550 ms,
  but USB Loader GX still returned to the System Menu. This rejects SCSI-level
  write protection alone as the cause; the independent DOS read-only file
  attributes remained unchanged. The overlay, device-mapper preload,
  packaging, and validator changes were removed; the strict read-only gadget
  was restored while both traces were retained.
- A new read-only live-export audit found that the server catalog had changed
  from the previously verified 1,284,936,704-byte single-file image to a
  3,619,423,232-byte four-file image with no FAT free space. All four WBFS
  files were copied through mutual-TLS NBD and independently checked with
  Wiimms ISO Tool 3.01a. Three pass; one reports an H0 integrity failure in its
  data partition. The next step is to identify whether that damaged payload
  was the attempted game. If not, a Wii `sysCheck.csv` is required to capture
  the exact base IOS assigned to installed d2x slots 248 through 251.
- The operator identified the failed launch as **10 Minute Solution**. Its
  catalog entry is one of the three WBFS files that passed full verification,
  not the file with the H0 integrity error. Payload corruption is therefore
  eliminated for this launch. The remaining evidence needed is a Wii
  `sysCheck.csv`, because the reported current d2x revision does not reveal
  which IOS base is installed in each of slots 248 through 251.
- The Wii SD card's SysCheck HDE report confirms the expected d2x-v11-beta3
  inventory: slots 248[38], 249[56], 250[57], and 251[58]. The cIOS
  installation is therefore current and correctly based.
- USB Loader GX r1283 was launching this game through its inherited global
  slot 249/base 56 configuration. Applied one reversible per-game comparison:
  the game's `GXGameSettings.cfg` entry now explicitly selects slot 250/base
  57. Alternate-DOL selection resolves to off for this title, other settings
  are unchanged, and no system IOS was installed or modified.
- The Wii SD card's two redundant FAT copies initially differed while each
  remained structurally intact. Reconciled them with `fsck.vfat`, then a
  non-mutating verification passed cleanly. A read-only remount confirmed both
  the SysCheck report and the explicit IOS250 game setting before the reader
  was unmounted and powered off.
- The controlled IOS250/base57 launch returned to the Wii System Menu just as
  the original IOS249/base56 launch did. This rules out the two standard d2x
  game bases as the differentiator. The title has no entry in USB Loader GX's
  built-in alternate-DOL table, so its default setting resolves to off. The
  remaining installed USB-base comparison is IOS251/base58.
- IOS251/base58 also returned to the System Menu. The operator noted a more
  discriminating symptom: this title's normal animated selection banner stays
  blank, and launch gives a brief green flash before returning.
- Reconnected to the current live export read-only through mutual TLS and
  copied the exact attempted WBFS. Both partitions pass WIT v3.05a
  verification and the FAT file occupies one contiguous cluster range.
  Extraction produced a valid 221,872-byte `opening.bnr` with IMET and U8
  signatures and banner/icon/sound members, a structurally valid
  7,101,888-byte `main.dol`, and a TMD that requests IOS53. The source image
  does contain the banner and boot material. A second verified title's banner
  is the next control for distinguishing SD banner-cache state from cIOS
  access to game-internal data through the gadget.
- American Mensa Academy, the second verified title, also displayed a blank
  banner and returned to the System Menu. The brief transition resembles a
  tiled/corrupted Wii-logo framebuffer rather than a game-generated error
  dialog.
- Read-only FAT allocation inspection found 4 KiB clusters and one contiguous
  extent per file. The two verified failures occupy approximately
  1.72–2.92 GiB and 2.92–3.37 GiB of the virtual disk. The verified Squeakquel
  file occupies approximately 0.93–1.72 GiB and is the only clean current
  payload entirely below 2 GiB, making its banner/launch the precise low-LBA
  control. The damaged similarly named file at the start of the disk is
  excluded from testing.
- The low-LBA Squeakquel control also displayed a blank banner and returned to
  the System Menu, eliminating a 2 GiB or high-LBA failure.
- Inspected USB Loader GX r1283's exact WBFS paths. Header enumeration can use
  read-only file access, but banner extraction and game boot call
  `split_open`, whose non-creation path still requests `O_RDWR`. Independent
  `mattrib` inspection of the live export confirmed that all four synthesized
  WBFS entries carry DOS read-only attribute `0x01`. This precisely explains
  why the loader can list titles but cannot reopen them for banner or boot
  access. The earlier writable block overlay retained these file attributes,
  so its failure did not test this condition.
- Changed only the synthesized WBFS file attribute from read-only `0x01` to
  archive `0x20`. The source library, NBD protocol export, and Pi USB LUN
  remain immutable/read-only. Added direct FAT-directory regression coverage
  and an independent `mattrib` assertion. Targeted tests, the complete
  unit/static suite, FAT verification, whitespace checks, and the hardened
  container lifecycle pass. A replacement TrueNAS image must be published and
  deployed before the physical banner and game-launch retest.
- Committed the repair as `74dfb5b` and published the replacement image as
  `ghcr.io/mhilton7/wiibridge-host:0.1.0-rc.1-wbfsattrfix.1@sha256:37d94d0c3f11ae8c33f96490fe9c0902b6b417907afd56a14d8dd370f4b2fe80`.
  The registry resolves the digest without stored credentials. TrueNAS
  redeployment and live attribute verification are the next controlled steps.
- After TrueNAS redeployment, strict HTTPS health returned healthy version
  `0.1.0-rc.1`. A fresh mutual-TLS connection reported the expected
  3,619,423,232-byte NBD export and independently confirmed it remains
  read-only. A read-only libnbd FUSE inspection showed archive `A` without
  read-only `R` on all four live WBFS entries, proving the replacement builder
  is active. The Pi controller at `192.168.0.181:9443` was unreachable at that
  point, so a clean Pi reconnect precedes the Wii banner/launch retest.
- The repaired physical path passed. USB Loader GX now displays the 10 Minute
  Solution banner, and the game loads successfully through the Zero W bridge
  and read-only TrueNAS export. Successful enumeration before the repair,
  failed banner/boot access with DOS read-only entries, and immediate success
  after changing only those entries to archive attributes confirm the root
  cause end to end. Sustained gameplay and a later reconnect cycle remain.

## 2026-07-27 — Complete GameCube library no-copy backend

- Confirmed schema 1 summed every disc's physical size, allocated and formatted
  a complete `library.img`, copied ISO/GCM/CISO or every FST file into it, and
  retained multiple payload-bearing generations.
- Replaced the complete-library path with schema 2 compact FAT32 metadata plus
  a sorted source-extent map. NBD reads resolve metadata, immutable source
  ranges, and zero padding without staging complete payloads. A bounded
  read-only handle cache performs identity checks and closes on backend close.
- ISO and GCM map as `game.iso`/`disc2.iso`, CISO maps as
  `game.ciso`/`disc2.ciso`, and extracted FST files map individually to their
  validated original files. FST tree changes invalidate activation.
- Physical memory-card mode is fully read-only. Emulated mode now fails
  configuration explicitly because a bounded save-only overlay is not yet
  implemented; it never falls back to a copied image.
- Schema-1 copied generations are detected and reported but never deleted
  automatically. Offline, explicitly targeted cleanup is documented.
- Measured synthetic fixture: 6,291,456 source bytes, 8,589,934,592 apparent
  virtual bytes, 2,252,800 physically allocated generation bytes, three mapped
  files/extents, and zero overlay bytes. Evidence is recorded in
  `reports/gamecube-no-copy-storage.json`.
- `make test`, `make static`, `make compose`, `make oci`,
  `go test -race ./server/host-daemon/...`,
  `go vet ./server/host-daemon/...`, the many-extent benchmark, and
  `git diff --check` pass. Exact repository-root `gofmt -w .` and
  `go mod tidy` encounter pre-existing unreadable root-owned Pi firmware
  artifacts under `build/pi-gen-zero-w-armhf`; scoped Go formatting passes and
  no module-file change is required.
- No physical GameCube Wii/USB Loader GX/Nintendont test hardware was available:
  `DEFERRED_HARDWARE_UNAVAILABLE`.
