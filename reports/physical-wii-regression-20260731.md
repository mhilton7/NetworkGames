# Physical Wii performance and loading regression report

Status: correction validated and published; TrueNAS deployment and
post-correction physical acceptance pending.

## Identity and environment

- Repository base: `8e8ec4014e9d2bf0e85d63cad3241f4e39613c33`.
- Repair branch: `agent/diagnose-physical-regressions`.
- Initially published/live image:
  `ghcr.io/mhilton7/wiibridge-host@sha256:aa1a5a6db11320c504daed2c8b198b1fb8bc6836c3a5ef90fb5b4c4adc44707f`.
- Live Host binary revision: `8e8ec4014e9d2bf0e85d63cad3241f4e39613c33`;
  build time `2026-07-30T03:39:38Z`; `/healthz` healthy and `/readyz` ready.
- Corrected image:
  `ghcr.io/mhilton7/wiibridge-host:sha-2851a568f0f6af1aacd24150e5e6c10d035a154b@sha256:dc9a1b7b9223efca9dd174e3c8129e28330d7ded20a95d7ea2039db2b1e8bab5`.
- Corrected revision: `2851a568f0f6af1aacd24150e5e6c10d035a154b`;
  build time `2026-07-31T21:33:14Z`; independently pulled binary and OCI
  revision agree; `dirty false`.
- Pi: Raspberry Pi Zero W Rev 1.1, target `zero-w-armhf`, clean firmware
  revision `c60d4b500944044a581e039570d3a8432b1921e2`, build time
  `2026-07-28T22:59:31Z`.
- Loader: USB Loader GX r1283. Wii system version and active cIOS slot/base:
  `NOT_CAPTURED`; no cIOS was changed.
- Three named legal control titles and their WIT integrity receipts:
  `NOT_CAPTURED_SOURCE_DATASET_NOT_MOUNTED_ON_WORKSTATION`. No source was
  treated as verified merely because its virtual directory entry exists.
- TrueNAS version, Apps backend, running container ID/digest, configured
  digest, start time, restart count, and host process/container counters:
  `UNVERIFIED_MANAGEMENT_ACCESS_UNAVAILABLE`. Runtime binary identity was
  verified independently; these management facts are not inferred.

## Baseline physical evidence

- The earlier 32 KiB live export used `0xffffffff` for both FSInfo fields.
  r1283's bundled libfat responds by walking the complete FAT during mount.
- After exact FSInfo deployment, USB Loader GX physically advanced beyond
  `Initializing USB devices`, confirming that the mount-scan defect was fixed.
- At `Loading resources`, Pi NBD completed requests advanced from 4,251 to
  4,908 over 27 seconds at approximately 1.2–1.5 MB/s, then reached 5,518 and
  stopped. Pi CPU returned below 1%. NBD read failures, reconnects, USB resets,
  and recent errors remained zero.
- Catalog-visible timing was not reached, so no successful cold/warm duration
  is claimed. The reported blank-banner and post-menu freeze symptoms remain
  pre-correction physical failures; exact per-title timestamps were not
  captured in the available session.
- A later idle sample on 2026-07-31 held at 49 completed NBD requests across
  13 seconds with zero throughput/failures/reconnects/resets while USB remained
  configured. This proves configured state alone does not imply useful reads.

## Exact live export audit

- Disk size: 1,813,217,599,488 bytes.
- Partition: MBR type `0x0c`, start 2,048, length 3,541,438,576 sectors;
  signature `0x55aa`; deterministic nonzero disk signature `0x25bc05e4`.
- FAT32: 512-byte sectors, 64 sectors/cluster (32 KiB), 32 reserved sectors,
  two FATs, 432,200 sectors/FAT, root cluster 2, 55,321,471 data clusters.
- Primary and backup boot sectors: byte-equal.
- Primary and backup FSInfo: byte-equal; valid signatures; free count 1;
  next-free cluster 55,321,472; advertised FAT entry zero.
- FAT copies: complete streaming comparison equal.
- Independent tools: `blkid` identifies FAT32 with 32 KiB filesystem blocks;
  `mtools` reads root and `/wbfs` and enumerates 987 virtual segments.
- Attributes: 987/987 archive set; 0/987 DOS read-only; 0 hidden; 0 system.
- Current full-segment size: 4,294,963,200 bytes (`4 GiB - 4 KiB`).
  Affected live segments: 59.

## Root causes and correction

1. Confirmed catalog-mount delay cause: unknown FSInfo sentinels triggered a
   capacity-proportional libfat scan. The already deployed exact-FSInfo change
   fixed USB initialization as a distinct physical result.
2. Confirmed format defect in the remaining image: a `4 GiB - 4 KiB` virtual
   file rounds up to exactly 2^32 allocated bytes on 32 KiB clusters. The FAT
   chain exists, but 32-bit chain-length accounting wraps to zero.
   `fsck.fat -n` reproduces the zero-chain result on the live export and a
   compact synthetic fixture. USB Loader GX r1283 itself uses
   `4 GiB - 32 KiB`, explicitly one cluster below the boundary.
3. Smallest correction: change only the virtual WBFS full-segment boundary to
   4,294,934,528 bytes. Source files, source offsets, NBD/TLS, LUN read-only
   enforcement, caching, Pi firmware, and cIOS remain unchanged.

The live archive-only attributes reject a recurrence of the earlier DOS
read-only regression. The corrected split boundary is a locally verified root
cause candidate for the post-enumeration, banner, and large-game failures, but
those symptoms are not reported fixed until the candidate is deployed and the
physical matrix passes.

## Rejected or separated hypotheses

- Generic TrueNAS/network bottleneck: rejected for the captured stall because
  request counters stopped and error/reconnect/reset counters stayed zero.
- Pi CPU/memory/temperature failure: rejected for the captured stall; CPU
  returned idle, and a later sample showed about 106 MB of 448 MB used and
  31–32 °C.
- Live FSInfo regression: rejected on the current image; both copies are exact.
- Live FAT-copy divergence or missing chains: rejected; copies compare equal
  and `mshowfat` finds the target chains.
- Live DOS read-only attribute regression: rejected; all 987 segments are
  archive-only.
- Telemetry on the critical path: not causal in the no-request stall. Atomic
  metric observation is allocation-free; enabled NBD benchmark overhead is
  about 90–100 ns/request on this host. Physical enabled/disabled Pi testing is
  still pending.
- SD corruption/cache state: the dirty/divergent Wii SD was repaired and its
  caches were reversibly isolated. A physical clean-cache result has not yet
  been supplied, so it remains a separated pending experiment.

## Local regression and validation results

- Pre-correction focused test: `FAIL` with 4,294,963,200-byte segments and
  `fsck.fat` zero-byte chains.
- Post-correction focused split/FSInfo/FAT/attribute/LFN/alias/source-byte
  tests: `PASS`.
- `make test`: `PASS` after updating the prior hard-coded split expectation.
- `go test -race ./server/host-daemon/...`: `PASS`.
- `go vet ./server/host-daemon/...`: `PASS`.
- `go test -v ./tests/fat32`: `PASS`.
- `make static`: `PASS`.
- `make compose`: `PASS`.
- `./tests/truenas/container-test.sh`: `PASS` on the dirty diagnostic build;
  it is not a release artifact.
- `make oci`: diagnostic manifest
  `sha256:5ccac175a103c8d644edbae7a36f1ff2da5539b8afc3194feb48fff3e8ac5ffe`;
  dirty worktree, not publishable.
- Clean committed local OCI layout: revision `f9fc32c`, `dirty false`, manifest
  `sha256:477ba2e3aa7322e9e04ce9facd7fad056ed3ef0d902ac3892531a77a1fca072e`.
- GitHub pull-request build at `f9fc32c`: `PASS`; main publication workflow at
  merge revision `2851a568`: `PASS`; immutable GHCR digest
  `sha256:dc9a1b7b9223efca9dd174e3c8129e28330d7ded20a95d7ea2039db2b1e8bab5`
  independently pulled and verified.
- Runtime identity mock pass plus expected digest-mismatch and
  missing-capability failures: `PASS`.

## Focused benchmarks

Host: AMD Ryzen 9 9950X3D, Linux amd64, Go 1.25.12. Three samples unless noted.

- 512 GiB FAT synthesis: 244–250 µs/op; about 288 KB/op; 2,134 allocs/op.
- On-demand FAT-sector read: 640–664 ns/op; 771–800 MB/s; one bounded
  512-byte allocation.
- Wii 1 MiB source read: 29.4–29.8 µs/op; 35.2–35.7 GB/s host-cache result.
- 10,000-extent lookup: 10.13–10.20 ns/op; 0 B/op; 0 allocations.
- NBD 64 KiB read path, metrics disabled: 936–952 ns/op.
- NBD 64 KiB read path, metrics enabled: 1,039–1,043 ns/op.
- Atomic NBD observation enabled: 62.01–62.45 ns/op, 0 allocations;
  disabled: 0.866–0.872 ns/op, 0 allocations.
- Cached Pi metric sample: 31.64–31.89 ns/op, 0 allocations; JSON endpoint:
  2.17–2.19 µs/op, 1,970 B/op, 13 allocations.

These host-cache benchmarks establish bounded overhead; they do not substitute
for physical Pi Zero W or Wii throughput.

## Deployment and rollback

Publication is complete; deployment is pending. Before replacement, retain the
current digest shown above and export the active TrueNAS YAML/configuration.
With the Wii powered off and USB/NBD detached, replace only the image reference
with the corrected digest-pinned reference above, recreate the app, run
`deploy/truenas/verify-runtime-identity.sh`, re-audit the regenerated live FAT
and split sizes, then reconnect NBD and USB in order.

If readiness, live metadata, or physical acceptance fails: detach USB,
disconnect NBD, restore
`ghcr.io/mhilton7/wiibridge-host@sha256:aa1a5a6db11320c504daed2c8b198b1fb8bc6836c3a5ef90fb5b4c4adc44707f`,
restore the prior YAML/configuration and active snapshot if required, recreate
the app, verify readiness, reconnect the unchanged Pi identity, and retain the
failed candidate evidence.

## Physical acceptance still required

- Three cold and three warm catalog timings with request/byte/FAT-sector counts.
- Animated banners: 3/3 verified titles.
- Three verified games through at least 20 minutes and a later loading event.
- No unexpected USB reset, NBD reconnect, source failure, or payload mismatch.
- Detach/disconnect/reconnect/reattach and one repeated game launch.

No post-correction physical success is claimed.
