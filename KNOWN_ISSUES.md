# Known issues and external facts

- The TrueNAS target at `192.168.0.175` is network-reachable: TCP 8445 and
  10809 accept connections, and the current HTTPS listener returns a successful
  health response. Strict verification now accepts its replacement server
  certificate for `192.168.0.175`, and the live NBD export accepts the
  generated Pi client identity and reports a read-only 1,284,936,704-byte
  disk. No authenticated TrueNAS management shell or API is connected, so the
  exact version, Apps backend, datasets, and ACL configuration remain unknown.
- The original generated server certificate covered only the Pi-facing
  `192.168.0.175` address while the Compose health check uses
  `https://127.0.0.1:8445/healthz`. That identity mismatch can leave the
  TrueNAS application unhealthy or not started after installing the new
  certificate. The generator now adds both identities, and a replacement
  server certificate/key under the same CA passes the packaged host's exact
  loopback health-check path. The replacement is now live.
- Docker Engine and Compose were installed on the build host and the hardened
  container and Compose parser gates now pass.
- NBD is pinned to mutual-X.509 TLS 1.2 for interoperability with the release
  host's libnbd 1.22/GnuTLS 3.8.9 client. That client fails to select its
  certificate for the equivalent Go TLS 1.3 request; HTTPS remains TLS 1.3.
- Physical Raspberry Pi Zero W testing became available on 2026-07-24. The
  original `0.1.0-rc.1` image booted but failed
  `wiibridge-recover.service`, produced no setup access point, and left no
  usable login route. Inspection found three release-blocking defects: detach
  incorrectly required pre-existing bridge configuration, the recovery AP and
  provisioning flow had not been packaged, and `/etc/wiibridge` prevented
  the controller service user from reading its credentials.
- Those defects are repaired in the working tree and on the physical test
  card. The ARMv6 controller, systemd units, unprovisioned detach path, device
  identity preservation, and card contents pass offline checks. A physical
  Zero W reboot confirmed the recovery detach and controller now start.
- That reboot also proved NetworkManager 1.52.1's `wpa_supplicant` AP backend
  cannot install a WPA key through the Zero W BCM43430 driver. The persistent
  journal recorded `nl80211: kernel reports: key setting validation failed` and
  an AP activation timeout. AP mode now uses a dedicated ARMHF `hostapd`
  process with `dnsmasq`; NetworkManager remains responsible for normal client
  Wi-Fi. The package, configuration, service graph, DHCP syntax, and preserved
  card identity pass offline validation.
- Physical hostapd retesting passed iPhone association and the complete WPA2
  RSN four-way handshake. The phone then waited for an address because the
  hardened dnsmasq unit could not write its default
  `/var/lib/misc/dnsmasq.leases` file through `ProtectSystem=strict`.
  Dnsmasq now runs as its unprivileged account with bounded network
  capabilities and a systemd-managed writable lease directory under `/run`.
- The following physical retest still produced the link-local iPhone address
  `169.254.19.116`. Persistent logs proved that dnsmasq could not traverse the
  mode-0750 `/etc/wiibridge` credential directory to read its otherwise
  valid configuration. The non-secret configuration now lives at
  `/etc/wiibridge-dnsmasq.conf` with root ownership and mode 0644; secrets
  remain protected under `/etc/wiibridge`. The configuration parses
  successfully when executed under the card's actual dnsmasq UID/GID, the
  service graph validates offline, and device/AP identity is unchanged.
  The subsequent physical test obtained DHCP and loaded the HTTPS login page,
  so this repair is physically verified.
- Manual authentication with the original 64-character management credential
  remained unsuccessful. New images now generate a 12-character lowercase
  hexadecimal management password, and the controller accepts that bounded
  minimum while retaining constant-time comparison and per-client rate
  limiting. The live card has the new ARMv6 controller and a newly generated
  12-character credential that exactly matches `WIIBRIDGE-SETUP.txt`; its
  AP password, device TLS identity, and machine identity are unchanged. The
  fresh-image login with the shorter password passed by reaching and submitting
  the authenticated network-setup form.
- That first network-setup submission exposed another systemd confinement
  defect: the root provisioning helper inherited the controller service's
  `ProtectSystem=strict` mount namespace and could not create its temporary
  NetworkManager client profile. The controller unit now grants write access
  only to the NetworkManager connection directory, `/etc/wiibridge`, and
  `/boot/firmware`, in addition to its existing runtime directory. The failed
  attempt left no partial profile or provisioned marker. The fix passes the
  full automated suite and is installed on the card with its credentials and
  identity preserved. The physical retest passed: the Pi saved the client
  profile and joined the configured home network.
- The physical test card was subsequently reported corrupted. A completely
  fresh Zero W image containing all working-tree repairs was built and flashed.
  All 163 allocated image ranges match on physical readback, both flashed
  filesystems pass non-destructive checks, and no current device I/O error was
  observed. Because bmap skips unallocated sparse ranges, a whole image-region
  hash is not expected to match on reused media; allocated data was compared
  directly instead. The fresh first boot subsequently passed root expansion,
  identity generation, recovery, AP, hostapd, dnsmasq, and controller startup;
  hostapd reached `AP-ENABLED`, and dnsmasq opened the intended DHCP range.
  The generated boot setup file exactly matches the live device configuration.
  Client login remains pending. If corruption recurs after clean shutdowns,
  replace the SD card or reader before further firmware diagnosis.
- No physical Pi 4, Pi 5, Linux USB-host acceptance setup, Wii, USB Loader GX
  installation, or legal private game fixture is currently available. Those
  corresponding hardware gates remain deferred or pending. A syntactically
  valid authorized VID/PID pair is present on the physical Zero W test card;
  its values are intentionally not recorded in public build notes.
- The USB Device Controller can be discovered automatically, but an authorized
  VID/PID cannot be inferred from the Wii or assigned safely by the firmware.
  The opt-in validated boot attachment and browser action controls pass
  automated checks and are installed and offline-verified on the live card.
  Offline inspection confirmed that the pair and auto-attach marker are
  present. The attach failure occurred because the helper sourced but did not
  export the pair to the child gadget process; that export is now fixed and
  installed. Physical attachment remains to be retested.
- The first physical server-connect action failed with
  `modprobe: FATAL: Module nbd not found`. The matching Zero W image does
  contain `nbd.ko.xz` for the running `6.18.34+rpt-rpi-v6` kernel and lists it
  in `modules.dep`; the controller's `ProtectKernelModules=yes` sandbox
  intentionally hid `/lib/modules` from its root helper. The repair preloads
  NBD through `systemd-modules-load` before the controller starts, keeps the
  kernel-module protection enabled, and makes the helper require the preloaded
  module and `/dev/nbd0`. Source/static/systemd checks pass. The repair is
  installed and offline-verified on the physical card with device state
  preserved. The rebooted Pi passed this layer and reached TLS negotiation.
- The subsequent physical connection attempt failed because Pi `nbd-client`
  could not load the installed `/etc/wiibridge/client.crt` and
  `client.key`. Offline inspection showed that the CA, client certificate, and
  client key on the card were all zero bytes after another unclean power
  removal. The generated bundle itself parses with OpenSSL and GnuTLS, has a
  matching key, validates for client authentication, and completes mutual TLS
  against the live TrueNAS export. The exact bundle is now installed and
  offline-verified on the card. Provisioning also verifies the submitted
  client certificate against the submitted CA before saving.
- To prevent another hot-removal corruption, the authenticated dashboard now
  includes a CSRF-protected, typed **Safely power off Pi** action. It detaches
  USB, disconnects NBD, flushes storage, and requests a non-blocking systemd
  poweroff through one exact sudo rule. Source tests, ARMv6 build/QEMU smoke,
  live-card file comparison, and systemd validation pass. Physical use and
  post-repair connection remain to be tested.
- The repaired physical TLS retest passed certificate loading and negotiation
  and received the expected 1,225 MB export size. Persistent journal inspection
  proves the first generic-netlink setup also succeeded: `/dev/nbd0` connected,
  acquired the correct capacity, and exposed partition `p1`. The deployed
  TrueNAS image then closed the healthy idle transmission after about 33
  seconds because its 30-second negotiation deadline remained active during
  transmission. Later retries encountered the stale occupied device and
  produced the misleading setup error. The server now clears the deadline upon
  entering transmission, and the Pi helper now cleans up stale state before
  reconnecting. The replacement server artifact passes regression and
  container tests. The idlefix deployment and physical Pi idle retest now pass.
- Windows successfully enumerated the repaired Zero W gadget and the USB
  controller reached `configured`, but requested formatting. Read-only live
  export inspection isolated this to zero sectors-per-track and head-count
  fields in the synthesized FAT32 BPB; `fsck.vfat` had accepted the malformed
  geometry while Windows and `mtools` rejected it. The builder now records
  conventional 63/255 geometry and the 2,048 hidden partition sectors.
  Independent `mtools` directory reading, `fsck.vfat`, unit, static, and
  whitespace checks pass. The cumulative `idlefix.2` container is deployed;
  its live BPB geometry and root/`wbfs` directory reads pass independently.
  The physical Windows remount still requested formatting. Persistent Pi logs
  show uninterrupted NBD and USB service with no host-read errors, isolating a
  second Windows-specific defect: the gadget is a fixed, read-only MBR disk,
  but its disk signature was zero, so Windows could not establish the required
  unique disk identity or persist one itself. The builder now derives a
  deterministic nonzero per-catalog MBR signature and matching FAT volume ID.
  Source and hardened-container tests pass, and idlefix.3 is deployed. The live
  export passes nonzero disk identity, matching volume ID, FAT checks, and a
  kernel read-only mount. Windows now detects and visually presents the
  idlefix.3 partition, resolving the reported partition-recognition failure.
  An explicit File Explorer `/wbfs` read has not yet been recorded. The user
  must not accept any Windows format prompt.
- The first Wii hardware attempt caused USB Loader GX to crash during startup
  with the bridge connected. Windows recognition means this is tracked as a
  distinct Wii/loader compatibility failure, not a recurrence of the FAT32
  identity defect. The exact USB Loader GX revision, d2x cIOS inventory, crash
  screen, physical USB port, loader/config storage location, and Pi journal
  are not yet available. The Pi is powered by the Wii and requires time to
  boot, connect NBD, and attach its gadget, so a controlled retry will wait 90
  seconds at the Wii Menu before launching the loader. No disk-format or
  gadget-mode change is justified until that result is captured.
- A controlled follow-up reaches the game catalog: USB Loader GX mounts the
  virtual FAT32 filesystem and displays the game file, then crashes when boot
  is requested. Immediately afterward the Pi controller still reports the NBD
  connection active, gadget attached, and USB link configured. This closes
  basic Wii enumeration and shifts the open defect to the game-boot/cIOS
  handoff, a transient USB reset not visible in the status snapshot, or the
  source WBFS payload. The exact crash mode, loader revision, selected game
  IOS, cIOS inventory, and persistent Pi journal still need capture.
- The captured attempt used USB Loader GX v4.0-r1283 and returned cleanly to
  the Wii System Menu when boot was requested. Slots 248 through 251 are
  installed, although their exact d2x version and base mappings are not yet
  independently captured. Journal evidence shows two cIOS takeover
  re-enumerations, including DWC2 control-endpoint halt warnings, without an
  NBD disconnect, block-read error, or network failure. The live WBFS file is
  one contiguous FAT extent, and an independent read-only verification passes
  both its update and data partitions. Disabling optional mass-storage bulk
  endpoint stalls did not change the physical outcome: the next launch still
  returned to the Wii System Menu. That experiment is rejected and the normal
  gadget behavior has been restored in source and on the card. A temporary
  boot-time function/event trace was installed to capture mass-storage command
  sequencing, DWC2 halt handling, and NBD request issue/completion. The first
  instrumented run reproduced the reboot, but its trace file remained empty:
  repeatedly scanning tracefs function metadata consumed 78 seconds of Pi Zero
  CPU and never reached capture. The normal journal again shows USB reset and
  re-enumeration with no NBD, network, or read failure. The instrumentation now
  scans the function list once, logs initialization immediately, and uses
  systemd readiness notification so USB auto-attach cannot start before
  capture is active. The corrected capture produced 1,749 lines. The real NBD
  session completed every traced request, all block errors belonged to the
  intentional pre-attach disconnect test, and cIOS continued reading through
  the handoff. It issued five write commands, but the read-only mass-storage
  function rejected each before accepting a payload; the final rejection was
  followed about 596 ms later by the reset that returned to the System Menu.
  Earlier rejected writes were survived, so this was treated as a controlled
  compatibility hypothesis rather than final proof. The comparison used a
  32 MiB, tmpfs-backed, nonpersistent device-mapper snapshot. It accepted all
  five writes, 390 reads completed, and all 117 traced NBD requests received
  complete replies; the Wii nevertheless returned to the System Menu. This
  rejects SCSI-level write protection alone as the launch failure. The test
  did not remove the independent DOS read-only attributes on every WBFS file,
  and the overlay has been removed from source and the card.
- The live catalog changed after the original single-file verification. Its
  current export is 3,619,423,232 bytes and contains four WBFS files filling
  the synthesized FAT volume. A fresh read-only copy and Wiimms ISO Tool 3.01a
  audit passes three files, while one fails an H0 integrity check in its data
  partition. The operator identified the attempted game as **10 Minute
  Solution**; its catalog entry is one of the three that passed, so the damaged
  file did not cause this launch failure. The damaged file should still be
  replaced from the operator's clean, legal backup. The Wii `sysCheck.csv`
  confirms d2x-v11-beta3 in slots 248[38], 249[56], 250[57], and 251[58], so
  the cIOS installation itself is correct. USB Loader GX had this game using
  its global slot 249/base 56 path. A narrow per-game setting now forces slot
  250/base 57 for the next physical comparison; no system IOS was changed.
  That comparison also returned to the System Menu, ruling out both standard
  d2x game bases. No title-specific alternate-DOL requirement is documented,
  and USB Loader GX's default alternate-DOL lookup resolves to off for this
  game. Slot 251/base 58 produced the same result. The operator additionally
  reports that the normal animated title banner remains blank and launch
  produces a brief green flash before returning to the System Menu.
- A fresh read-only copy of the attempted game from the live mutual-TLS NBD
  export passes both partition verifications. Its FAT allocation is one
  contiguous extent; `opening.bnr` extracts at 221,872 bytes with valid IMET
  and U8 signatures plus `banner.bin`, `icon.bin`, and `sound.bin`; `main.dol`
  is a valid 7,101,888-byte DOL; and the TMD normally requests IOS53. The blank
  banner is therefore a Wii-side runtime/cache symptom rather than a missing
  server file. Selecting another verified title and observing its banner
  before launch is the next control needed to separate a title-cache problem
  from the gadget/cIOS file-read path. That second control, American Mensa
  Academy, also had a blank banner and returned to the System Menu after a
  tiled/corrupted Wii-logo framebuffer flash.
- FAT extent inspection reveals that both verified failures occupy high disk
  offsets: 10 Minute Solution spans about 1.72–2.92 GiB and American Mensa
  Academy spans about 2.92–3.37 GiB. The only verified current file entirely
  below 2 GiB is Alvin and the Chipmunks: The Squeakquel at about
  0.93–1.72 GiB. Its banner and launch are the next low-LBA control. The
  similarly named Alvin and the Chipmunks file at the start of the disk must
  not be used because it is the payload with the independent H0 integrity
  failure. The low-LBA Squeakquel control also had a blank banner and returned
  to the System Menu, eliminating the offset theory.
- The shared blocker is now confirmed. Every synthesized WBFS FAT directory
  entry carries DOS attribute `0x01` (read-only). USB Loader GX r1283 uses a
  read-only `fopen` path to enumerate game headers, but its banner and boot
  paths call `split_open`, which opens the same WBFS file with `O_RDWR` even
  for reads. The file-level read-only attribute therefore explains successful
  enumeration followed by blank banners and pre-entrypoint return to menu.
  The builder now emits the normal archive attribute `0x20`; immutability
  remains enforced by both the read-only NBD export and Pi USB LUN. Internal
  byte-level and independent `mattrib` regressions, the full unit/static suite,
  FAT validation, and hardened container lifecycle pass. Commit `74dfb5b` is
  published as
  `ghcr.io/mhilton7/wiibridge-host:0.1.0-rc.1-wbfsattrfix.1@sha256:37d94d0c3f11ae8c33f96490fe9c0902b6b417907afd56a14d8dd370f4b2fe80`.
  The replacement is now deployed: strict HTTPS health passes, the
  3,619,423,232-byte mutual-TLS NBD export remains read-only, and independent
  live `mattrib` inspection reports archive `A` without read-only `R` on all
  four WBFS entries. After the Pi reconnected, USB Loader GX displayed the
  previously blank 10 Minute Solution banner and the game loaded successfully.
  This physically confirms the DOS read-only attribute as the shared banner
  and boot blocker. Sustained gameplay remains pending, and the separately
  damaged Alvin and the Chipmunks payload still needs replacement from a
  clean, legal backup.
- The required filename `CODEX_PRODUCTION_PROMPT.txt` was absent; the complete
  controlling content was present and read from
  `wiibridge_project_prompt.txt`.
- The firmware images were each built from a clean board work tree and passed
  offline validation, but a second complete build was not run to establish
  byte-for-byte reproducibility. The corresponding release gate remains
  `PENDING`; no reproducibility claim is made.
- Release provenance identifies the committed base and a source-tree digest,
  and explicitly records `sourceWorktreeDirty: true` because this implementation
  has not been committed.
- GitHub authentication is restored. The repair branch is published in draft
  PR #1, and the cumulative idlefix.1 image is available through GHCR. The
  Windows FAT32 follow-up remains on the same branch until its replacement
  image and physical retest complete.
- The schema-2 complete GameCube library currently supports
  `WIIBRIDGE_GAMECUBE_MEMORY_CARD_MODE=physical` only. `emulated` is rejected
  with a configuration error until a bounded save-only sector overlay can be
  implemented and validated; there is no silent fallback.
- Physical USB Loader GX/Nintendont testing of the new synthetic GameCube
  complete-library backend is `DEFERRED_HARDWARE_UNAVAILABLE`. Host-side FAT32
  inspection, exact source read-through, read-only behavior, and storage
  allocation tests pass, but physical compatibility is not claimed.
