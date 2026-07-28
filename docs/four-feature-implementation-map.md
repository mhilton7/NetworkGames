# Four-feature implementation map

Date: 2026-07-28

This note records the repository state at commit `ce73242` before the bounded
save overlay, compatibility negotiation, source reconciliation, and
performance-dashboard work.

## Current ownership

- GameCube generation and activation: `server/host-daemon/gamecube/library.go`
  builds schema-2 metadata and opens `fat32virtual.Backend`.
- FAT namespace and trusted extents:
  `server/host-daemon/fat32virtual/layout.go` assigns clusters and emits
  metadata/source extents; `backend.go` routes reads and currently rejects all
  writes.
- Legacy save support: `server/host-daemon/gamecube/saves.go` backs up and
  restores `.raw` files inside the retired per-title copied-volume path.
  `library.go` explicitly rejects complete-library emulated mode.
- NBD routing: `server/nbd-plugin/nbd.go` advertises backend read-only state,
  routes `READ`, `WRITE`, and `FLUSH`, and rejects trim/write-zeroes.
- Export switching: `server/host-daemon/exportprofile/manager.go` pins a
  backend for every NBD session. `server/host-daemon/main.go` and
  `bridgecontrol` retain the detach/disconnect/select/connect/attach order.
- Host/Pi reporting: build variables and `buildVersion` are in Host `main.go`;
  authenticated Pi status and actions are in `pi/controller/main.go` and
  `server/host-daemon/bridgecontrol`.
- Scanning: `scanner/scanner.go` owns Wii WBFS discovery and
  `gamecube/scanner.go` owns GameCube discovery. Both currently return a
  successful in-memory result or an error; traversal errors can be reduced to
  item rejections.
- Persistence: `server/host-daemon/store/store.go` has a version-1 SQLite
  snapshot schema. It does not yet persist source-root health or GameCube
  catalog state.
- Dashboard: Host routes and API handlers live in `main.go`; templates and
  static assets are under `server/host-daemon/web`.
- Metrics and diagnostics: `/metrics` currently exports catalog counts.
  Bounded GameCube and Wii benchmarks live beside their backends; JSON reports
  are in `reports/`; `scripts/performance-report` is an offline Wii benchmark.
- Deployment and firmware: `deploy/truenas`, the container Dockerfile and
  scripts, `pi/packaging`, `scripts/build-firmware.sh`, `versions.lock`, and
  `Makefile`.

## Integration design

### Save overlay format

Save-overlay format 1 stores authoritative cards below
`/data/gamecube/saves/{individual,shared}`. Each object has an `active.raw`,
immutable association metadata, a bounded block journal, and validated,
checksummed backups. A same-directory checkpoint is flushed and renamed
atomically; the previous active card remains authoritative until rename and
directory sync complete.

The schema-2 FAT builder accepts host-generated save files in addition to
source files. It emits an immutable writable-extent map containing virtual
offset, length, object ID, card offset/size, generation, and layout checksum.
The backend checks a complete write against exactly one such extent before
translation. FATs, directories, boot sectors, game data, padding, and
unallocated ranges remain non-writable. Physical mode does not create save
extents and retains the current read-only backend.

### Compatibility contract

`shared/compat` owns descriptor schema 1, stable capabilities, operation
requirements, structured errors, and protocol-range evaluation. Host and Pi
descriptors use linked build metadata. The Pi returns its descriptor and
bounded runtime metrics through authenticated management routes. Every
state-changing Host workflow performs a fresh authenticated probe and
operation-specific evaluation; cached results are display/diagnostic data
only.

### Source reconciliation

`shared/sourcehealth` defines stable source/item states and error codes.
SQLite migration 2 stores source-root identity, timestamps, counts, failures,
the last complete Wii/GameCube result, missing observations, and bounded
events. Scans have explicit preflight and complete/partial outcomes. A failed
or partial scan updates source health but never replaces the last complete
catalog. Absence becomes confirmed only after two complete available scans.
Runtime read failures emit a rate-limited event and metrics signal outside the
catalog mutation path.

### Metrics and sessions

`shared/perf` provides fixed atomic counters, fixed latency buckets, rolling
minute buckets, bounded warnings, and bounded session summaries. Backend,
save, NBD, Host runtime, compatibility, and source status feed the collector
without disk writes on request paths. SQLite stores only bounded session
summaries at detach/end. Low-frequency aggregate checkpoints use an atomic
replace beneath the dedicated performance data directory.

The Host exposes authenticated bounded performance APIs and one dashboard
section refreshed every five seconds. The bridge manager polls the Pi once per
configured interval and shares the cached sample across widgets. Missing Pi
telemetry degrades only Pi panels.

## Security and rollback boundaries

- `/library` remains read-only and source paths never derive from browser
  input.
- Save names are validated identifiers, all filesystem targets are confined
  beneath managed roots, and symlinks/special files are rejected.
- Browser mutations retain existing authentication, CSRF, and confirmation
  checks. Pi requests retain certificate pinning and the management token.
- Database migrations are additive. Existing active generations, cards,
  credentials, certificates, and snapshots remain readable by rollback
  binaries; no automatic legacy payload-generation deletion is introduced.
- Host-only tests do not establish Pi Zero W or physical-Wii performance.
