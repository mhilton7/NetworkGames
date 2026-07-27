# Codex Host Restore Report

## 1. Project path

Active project path:

`/home/tvos/NetworkGames-zero-w-boot-fix`

The active project was identified by its complete NetworkGames HostBridge layout,
including `pi/controller`, `pi/packaging`, `server/nbd-plugin`,
`deploy/truenas`, `scripts`, `Makefile`, `go.mod`, `BUILD_STATUS.json`,
`KNOWN_ISSUES.md`, `WORKLOG.md`, and `RELEASE_GATES.md`.

## 2. Original current commit and branch

Before restoration:

- Branch: `agent/fix-pi-provisioning-nbd-idle`
- Commit: `629ba29dd87680d38ded939464dc33c23b1c2057`
- Commit subject: `Record Zero W power-loss protection evidence`
- Upstream: `origin/agent/fix-pi-provisioning-nbd-idle`
- Remote: local checkout at
  `/media/tvos/8fe1920a-9d7c-4d69-a22e-6fe7c41e1faa/networkgames-livebridge`
- The checkout had tracked modifications and untracked protection-era firmware
  files. The complete original status, unstaged diff, staged diff, commit,
  branch, and recent log are stored with the safety snapshot.
- Available space before snapshot creation was approximately 55 GiB on
  `/home/tvos` and 122 GiB on the mounted external project filesystem.

## 3. Safety snapshot

Snapshot directory:

`/home/tvos/networkgames-before-restore-20260726-154457`

Archive:

`/home/tvos/networkgames-before-restore-20260726-154457/NetworkGames-zero-w-boot-fix.tar.xz`

SHA-256:

`73c34f9a553112474cbcc6180545ba564ba9a77d9e02076a1db104cc0b72c0a3`

The archive contains the entire 13 GiB protection-era project, including
`.git`, tracked files, untracked files, ignored build output, generated firmware,
logs, file modes, ACLs, and extended attributes. `tar -tJf` successfully listed
the archive. `AGENTS.md`, `.git/HEAD`, and a systemd unit were selectively
extracted, and the extracted `AGENTS.md` matched the original byte-for-byte.
`sha256sum -c` passed after creation and again immediately before restoration.

For immediate recoverability, the original unpacked directory was moved intact
to:

`/home/tvos/networkgames-before-restore-20260726-154457/NetworkGames-zero-w-boot-fix-protection-era-tree`

## 4. Backup candidates found

### Selected complete checkout

`/home/tvos/NetworkGames-pre-ethernet-20260725T091858Z`

- Complete Git checkout.
- Timestamp: 2026-07-25 02:19 PDT.
- Branch: `agent/fix-pi-provisioning-nbd-idle`.
- Commit: `f3a2cb2158819b640936d050ba3529bad4ddb7c1`.
- Commit subject: `Record successful physical Wii launch`.
- Contains all required source, packaging, service, deployment, test, and
  durable-status components.
- `git fsck --full` found no structural repository failure. It reported one
  harmless dangling blob.
- The only untracked source-tree file is
  `deploy/truenas/compose.ghcr.yaml`.

### Equivalent Git bundle

`/home/tvos/NetworkGames-pre-ethernet-20260725T091858Z.bundle`

- Valid bundle whose branch and HEAD resolve to the selected `f3a2cb2` commit.
- Kept as an independent recovery source.

### Rejected protection-era checkout

`/media/tvos/8fe1920a-9d7c-4d69-a22e-6fe7c41e1faa/networkgames-livebridge`

- Complete checkout, but HEAD is protection-era commit `629ba29`.
- Contains the `1c1f3d5` read-only-root/power-loss redesign and later evidence.
- Contains untracked pre-readonly reports, but those are not a complete source
  backup.

### Rejected protection-era Desktop backup

`/home/tvos/Desktop/NetworkGames-safe-backup-2026-07-25`

- `NetworkGames-history.bundle` resolves to protection-era commit `629ba29`.
- `NetworkGames-project-files.zip` identifies the same protection-era commit.
- It is useful as a safety artifact but is not the requested rollback source.

### Rejected SD-card backup

`/home/tvos/NetworkGames-card-backup-pre-bootfix-20260725`

- Contains `bootfs.tar`, `networkgames-etc.tar`, and `ngstate.tar`.
- This is device-state material, not a complete source checkout.
- It was preserved in place and not opened, copied into the source, or modified.

### Rejected incomplete prototype

`/home/tvos/NetworkGames/networkgames-bridge`

- Git checkout at initial commit `15d031f`.
- Does not contain the complete later HostBridge project structure or the
  physically Wii-verified implementation.

## 5. Selected backup and evidence

Selected source:

`/home/tvos/NetworkGames-pre-ethernet-20260725T091858Z`

Selected commit:

`f3a2cb2158819b640936d050ba3529bad4ddb7c1`

Selection evidence:

- The commit predates `1c1f3d5` (`Protect Pi firmware from sudden power loss`)
  and `629ba29` (`Record Zero W power-loss protection evidence`).
- `RELEASE_GATES.md` at `f3a2cb2` records successful Wii detection, USB Loader
  GX enumeration, banner extraction, and legal game launch.
- The `f3a2cb2` commit itself records the successful physical Wii launch.
- It contains the complete Pi controller, Pi packaging, NBD server, TrueNAS
  deployment, tests, and documentation.
- It retains the intended read-only host library, read-only NBD export,
  read-only ConfigFS mass-storage LUN, and immutable virtual-disk design.
- It does not contain `networkgames-overlayroot.conf`,
  `networkgames-fstab-extra`, `networkgames-state`,
  `docs/power-loss-protection.md`, or the protection-era initramfs/state
  machinery.
- A content search found no implementation of overlayroot, overlay-init,
  `systemd.volatile`, read-only root flags, an initramfs root overlay, or
  power-cut detection.
- The exact diff from `f3a2cb2` to protection commit `1c1f3d5` identifies the
  unwanted redesign boundary: 34 files, 608 insertions, including overlayroot,
  persistent-state, power-loss documentation, root/firstboot changes, image
  sanitation changes, and protection-specific validators.

The restored backup contains a pre-existing typed `poweroff` helper action. It
detaches USB, disconnects NBD, calls `sync`, and invokes ordinary systemd
poweroff. It was already part of the physically working backup and is not
OverlayFS, root remounting, shutdown interception, rollback, power-cut
monitoring, or the later protection firmware. It was preserved faithfully.

## 6. Files restored

The complete selected checkout was restored at the unchanged active path,
including:

- Git history and branch state at `f3a2cb2`.
- Raspberry Pi controller and cache implementation.
- Privileged helper and provisioning scripts.
- NBD connection, stale-device handling, disconnect, and auto-attach behavior.
- ConfigFS USB gadget creation and teardown.
- Read-only USB mass-storage LUN and validated USB VID/PID handling.
- Zero W, Pi 4, and Pi 5 pi-gen source configuration.
- Setup AP, hostapd, dnsmasq, NetworkManager provisioning, and recovery units.
- TrueNAS host daemon, scanner, virtual disk, and deployment definitions.
- Mutual TLS and certificate/key validation.
- Read-only NBD protocol implementation with the negotiation deadline cleared
  before long-lived transmission.
- Tests, validation scripts, release evidence, and the physically verified
  `f3a2cb2` durable status files.
- The backup's untracked `deploy/truenas/compose.ghcr.yaml`.

## 7. Files and feature groups removed from the active path

The protection-era checkout remains recoverable in the safety snapshot, but
these later feature groups are no longer present in the active source:

- Overlayroot configuration and read-only-root conversion.
- The added persistent `ngstate`/fstab protection machinery.
- Power-loss-protection documentation and validation evidence.
- Protection-era initramfs identity hooks.
- Root/state identity scripts and fixed-layout state checks.
- Protection-specific swap and build-ID additions.
- Later firstboot, journald, systemd, image sanitation, and offline-validation
  changes associated with the protected-root redesign.
- Generated protection-era firmware and build trees. These were not deleted;
  they remain in both the compressed safety archive and the moved original tree.

No WBFS library file was read, copied, renamed, rebuilt, or modified.

## 8. Preserved configuration, secrets, and local data

No live `.env`, private key, certificate, Wi-Fi profile, administrator token, or
device identity was found in the source tree outside its ignored/generated build
tree; `.env.example` is a non-secret template.

The following external data was deliberately left untouched:

- `/home/tvos/networkgames-credentials-server-192.168.0.175-pi-192.168.0.181`
- `/home/tvos/networkgames-card-backups`
- `/home/tvos/NetworkGames-card-backup-pre-bootfix-20260725`
- `/home/tvos/networkgames-usb-trace`

The protection-era `dist`, `build`, firmware images, diagnostics, and logs are
preserved in the complete safety snapshot and moved original tree. Nothing from
those locations was automatically copied back because that could reintroduce
the unwanted firmware configuration.

No certificate or private key was regenerated or replaced. No USB VID/PID was
invented. No host, TrueNAS, container, raw device, SD card, or Git remote was
modified.

## 9. Critical functionality inspection

The restored source coherently retains:

- NBD kernel-module boot configuration and a helper that refuses to continue
  when the module/device is unavailable.
- Stale and occupied `/dev/nbd0` detection and explicit disconnect handling.
- Read-only NBD flags and `blockdev --setro`.
- Clearing the NBD negotiation deadline before transmission so idle mounted
  sessions remain connected.
- ConfigFS gadget creation and teardown.
- `lun.0/ro` enforcement and backing-device read-only verification.
- Exported, syntax-validated USB VID/PID environment values.
- TLS CA, client certificate, and private-key parsing, matching, permissions,
  and installation.
- USB detach before NBD disconnect.
- Correct systemd module-loading and network startup ordering.
- Raspberry Pi Zero W board validation.
- Setup AP support through hostapd and dnsmasq.
- A writable runtime lease directory for dnsmasq.
- NetworkManager connection-profile write access in the controller unit.
- The read-only TrueNAS library and read-only WBFS virtual disk.

## 10. Validation commands and results

No deployment, live NBD connection, USB attachment, firmware build, SD-card
mount, raw-device write, container start, commit, push, or release publication
was performed.

### Safety and source integrity

- `sha256sum -c .../NetworkGames-zero-w-boot-fix.tar.xz.sha256`
  - Exit 0; passed.
- `tar -tJf .../NetworkGames-zero-w-boot-fix.tar.xz`
  - Exit 0; complete archive listing created.
- Selective archive extraction and `cmp` of `AGENTS.md`
  - Exit 0; passed.
- `git fsck --full` on selected backup
  - Exit 0; repository structurally valid; one harmless dangling blob reported.
- `git diff --exit-code f3a2cb2 -- .`
  - Exit 0; no tracked working-tree difference in the selected backup.

### Project validation

- `make test`
  - Exit 0.
  - All server, Pi controller, FAT32, integration, NBD, and unit tests passed.
- First `make static`
  - Exit 2.
  - Go vet and shellcheck passed, then `validate-pi-static.sh` correctly stopped
    because the source-only backup lacked its expected
    `build/bin/networkgames-host` prerequisite.
  - No source change was made.
- `make server`
  - Exit 0.
  - Built only the local host binary required by static validation; did not
    build firmware, deploy, or start a service.
- Second `make static`
  - Exit 0.
  - Go vet, shellcheck, staged-root systemd unit verification, Pi target
    configuration checks, NBD checks, ConfigFS/read-only checks, USB VID/PID
    export check, setup-AP checks, TLS provisioning checks, persistent journal
    check, and secret-pattern scan all passed.
- `make compose`
  - Exit 0.
  - Static Compose policy and Docker Compose parsing passed; nothing deployed.

### Additional host-context systemd diagnostic

- `systemd-analyze verify pi/packaging/systemd/*.service`
  - Exit 1 on the host source tree because target-image executables such as
    `/usr/libexec/networkgames-helper`, `hostapd`, and the Pi controller are not
    installed on this host.
  - It reported no unit syntax error.
  - The authoritative project validator subsequently staged the units and
    required executables into `build/systemd-verify-root` and its
    `systemd-analyze verify --root=...` check passed during the successful second
    `make static`.

Validation logs were retained under `/tmp/networkgames-restore-*.log` for this
host session.

## 11. Remaining warnings

- The restored source is the last physically Wii-verified source state, but this
  restoration did not rebuild or flash firmware.
- No current SD card or running Pi was inspected or changed.
- The external credentials and device-state backups were deliberately not
  merged into source.
- The restored branch name still references the historical provisioning/NBD
  work; no branch or history rewrite was performed.
- The selected Git repository contains one harmless dangling blob.
- The original `f3a2cb2` clean-shutdown helper remains because it is part of the
  selected working backup. It is not the later protected-root redesign.

## 12. Current Git status and diff summary

Current branch:

`agent/fix-pi-provisioning-nbd-idle`

Current commit:

`f3a2cb2158819b640936d050ba3529bad4ddb7c1`

Tracked source diff:

None.

Untracked files:

- `CODEX_HOST_RESTORE_REPORT.md` (this required report)
- `deploy/truenas/compose.ghcr.yaml` (present in the selected backup)

The validation-created `build` directory is ignored by Git.

## 13. Readiness and remaining work

The source restoration is complete and the non-destructive software validation
passed. The project is ready for a separately authorized Raspberry Pi firmware
rebuild from the restored `f3a2cb2` source.

Before any rebuild or hardware action:

1. Review this report and the selected commit.
2. Confirm which board target should be rebuilt.
3. Keep device-specific credentials and provisioning separate from the generic
   firmware image.
4. Explicitly authorize a firmware build if desired.
5. Separately identify and explicitly authorize any SD-card target before
   flashing.
6. Repeat physical Pi boot, NBD, USB gadget, Wii detection, and legal game
   launch acceptance tests after a rebuilt image is flashed.

RESTORE_COMPLETE_VALIDATION_PASSED
