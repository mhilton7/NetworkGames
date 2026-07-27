# GameCube implementation report

Date: 2026-07-26  
Branch: `feature/gamecube-support`  
Baseline: `64ff84b43700d96c0ba7ab495a006301a9ff2014`

## 1. Existing architecture discovered

The production Wii path is a read-only WBFS-backed synthetic FAT catalog
exported by mutual-TLS NBD, connected to `/dev/nbd0` on a Pi Zero W, and
presented through ConfigFS mass storage on the direct `dwc2` gadget port. No
host or Pi mount is involved during export.

## 2–3. Selected GameCube architecture and rationale

GameCube uses a separate content-addressed, persistent MBR/FAT32 disk image
with 32 KiB clusters and standard Nintendont paths. A session-pinned export
manager switches whole profiles only while disconnected. This isolates the
known-working Wii mapper, avoids copies on repeated launches, gives Nintendont
a normal filesystem, and permits narrowly scoped save writes.

## 4–5. Upstream pins

- Nintendont: `0f69235b99099ef2851a5f6b7b0e349c572237e8`
- USB Loader GX r1283:
  `e25c4f3501ed957b7db73f79c51fdf00715ab2e2`

No upstream fork or patch was made.

## 6–9. Implementation

- Read-only scanner for ISO, GCM, pinned uLoader CISO/CSO, and extracted FST.
- Header-derived ID/title/region/revision/disc metadata, SHA-256, validation,
  duplicate/orphan rejection, and metadata-based two-disc pairing.
- Atomic per-title FAT32 cache with `game.iso`/`disc2.iso` layout.
- All/Wii/GameCube host views, per-game settings, asynchronous import, and
  explicit export selection.
- State machine prevents simultaneous profiles, concurrent switches, and
  selection with active reads.
- Pi actions explicitly distinguish Wii read-only, GameCube physical-card
  read-only, and GameCube emulated-card writable mode.
- Atomic, validated, revision-separated save backups with at least five
  versions and managed restore.
- Reviewable official Nintendont SD package with hashes.

## 10. Filesystem and power safety

The source library, Wii export, and physical-card GameCube export remain
read-only. Emulated-save writes are limited to a separately selected GameCube
volume. The Pi never mounts an exported image. Existing shutdown, TLS,
credential, board, VID/PID, service-hardening, and USB detach-before-NBD
disconnect behavior was retained. No HAT/hub support, overclock, real-time
scheduling, OverlayFS, or safety weakening was introduced.

## 11. Performance

The selected image descriptor stays open and uses 64-bit positional I/O.
Bounded NBD read-buffer reuse changed the 64 KiB acquisition benchmark from
8.2–13.7 us, 65,536 B, and 1 allocation per operation to 7.795–7.840 ns,
0 B, and 0 allocations after warm-up. This is a host microbenchmark; physical
USB latency metrics remain pending.

## 12. Host validation

Passed:

```text
make test
make static
make server
make compose
make oci
go test -race (all source package sets)
GOOS=linux GOARCH=arm GOARM=6 static Pi controller cross-build
Nintendont package SHA-256 verification
```

The generated host binary SHA-256 was
`99929fdd670e87b0306709f68a15b3875f366635a6124239341eddcf355e60c6`.
The pre-document OCI build produced
`sha256:809452d1ef9ade3cf29fe42ac910536311dec23028317701b2feecc215ac07be`;
a final release image must be rebuilt from the reviewed final commit.

An unscoped `go test -race ./...` could not traverse old root-owned pi-gen
build artifacts under `build/`. The same race run over every source package
set passed. Those pre-existing build artifacts were not deleted or modified.

The pure-Go volume validator passed all synthetic FAT fixtures. `fsck.fat` is
not installed on this development host and is absent by design from the
scratch production container, so an independent `fsck.fat -n` release check
remains pending in a disposable validation environment.

## 13–14. Physical results and Wii regression

GameCube hardware: **not tested and not deployed**. No GameCube launch, save,
disc switch, USB timing, temperature, or ten-cycle claim is made.

Wii: all host/unit/integration/race regressions pass. The preserved baseline
records prior physical success for `SAVE5G` and 10 Minute Solution, but the
required post-change two-game Wii run has not occurred.

## 15. Remaining limitations and gates

- Explicit approval, final OCI/Pi image build, deployment, and physical Wii
  matrix are required.
- A legal single-disc, two-disc, and audio-streaming GameCube test set is
  required from the operator.
- Independent `fsck.fat -n`, Pi performance/throttling data, ten cold
  launches, ten mode switches, save reload/restore, and two Wii regressions
  remain open.
- Upstream Nintendont has no single top-level license file; redistribution
  review is required.
- ZIP, RVZ, NKit, and other non-Nintendont runtime formats are intentionally
  unsupported.

## 16–17. Deployment and rollback

Exact reviewed procedures:

- `docs/gamecube-deployment.md`
- `docs/gamecube-rollback.md`

Rollback anchor:
`64ff84b43700d96c0ba7ab495a006301a9ff2014`; full archive SHA-256:
`18c0e7f2897c6f4b3afbc3a9947a4c28fab552f98f7483376849385505b8e332`.

## Completion classification

- Completed and host-tested: scanner, cache, export state machine, UI/API,
  Pi mode logic, save safety, utilities, Nintendont package, build/static
  validation, and rollback documentation.
- Completed but hardware-tested only after deployment: none.
- Not completed: physical acceptance and post-change Wii regression.
- Blocked by unavailable operator-provided media/approval: physical Wii gate.

Final status: `IMPLEMENTATION_COMPLETE_HOST_VALIDATION_PASSED_HARDWARE_PENDING`
