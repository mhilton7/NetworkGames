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
