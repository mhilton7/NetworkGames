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
  `networkgames-recover.service`, produced no setup access point, and left no
  usable login route. Inspection found three release-blocking defects: detach
  incorrectly required pre-existing bridge configuration, the recovery AP and
  provisioning flow had not been packaged, and `/etc/networkgames` prevented
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
  mode-0750 `/etc/networkgames` credential directory to read its otherwise
  valid configuration. The non-secret configuration now lives at
  `/etc/networkgames-dnsmasq.conf` with root ownership and mode 0644; secrets
  remain protected under `/etc/networkgames`. The configuration parses
  successfully when executed under the card's actual dnsmasq UID/GID, the
  service graph validates offline, and device/AP identity is unchanged.
  The subsequent physical test obtained DHCP and loaded the HTTPS login page,
  so this repair is physically verified.
- Manual authentication with the original 64-character management credential
  remained unsuccessful. New images now generate a 12-character lowercase
  hexadecimal management password, and the controller accepts that bounded
  minimum while retaining constant-time comparison and per-client rate
  limiting. The live card has the new ARMv6 controller and a newly generated
  12-character credential that exactly matches `NETWORKGAMES-SETUP.txt`; its
  AP password, device TLS identity, and machine identity are unchanged. The
  fresh-image login with the shorter password passed by reaching and submitting
  the authenticated network-setup form.
- That first network-setup submission exposed another systemd confinement
  defect: the root provisioning helper inherited the controller service's
  `ProtectSystem=strict` mount namespace and could not create its temporary
  NetworkManager client profile. The controller unit now grants write access
  only to the NetworkManager connection directory, `/etc/networkgames`, and
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
  could not load the installed `/etc/networkgames/client.crt` and
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
  container tests, but still requires target deployment before physical retest.
- The required filename `CODEX_PRODUCTION_PROMPT.txt` was absent; the complete
  controlling content was present and read from
  `networkgames_wbfs_hostbridge_truenas_3pi_no_hardware_codex_prompt.txt`.
- The firmware images were each built from a clean board work tree and passed
  offline validation, but a second complete build was not run to establish
  byte-for-byte reproducibility. The corresponding release gate remains
  `PENDING`; no reproducibility claim is made.
- Release provenance identifies the committed base and a source-tree digest,
  and explicitly records `sourceWorktreeDirty: true` because this implementation
  has not been committed.
- GitHub publication is currently blocked by an invalid GitHub CLI token for
  the active `mhilton7` account. Run `gh auth login -h github.com` before
  retrying the publisher; no release upload has been recorded as passed.
