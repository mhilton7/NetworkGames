# Squeakquel diagnosis, deployment and rollback report

## 1. Exact symptom reproduced

Historical physical evidence records that USB Loader GX r1283 detected
`SAVE5G`, showed a blank animated banner, then briefly transitioned and
returned to the Wii System Menu before gameplay. The current task reproduced
the media and backend operations on the host, not the physical failure,
because the Pi SD card was attached to this host and no Wii was connected.

## 2. Root cause

The shared launch failure was caused by DOS read-only (`0x01`) attributes on
the synthesized FAT WBFS files. Loader enumeration used read-only access, but
banner/boot reopened the same file with `O_RDWR`. Commit `74dfb5b` changed
the virtual file attribute to archive (`0x20`) while retaining read-only NBD,
library and gadget LUN enforcement.

## 3. Proof

Verified low-LBA Squeakquel and high-LBA controls failed identically; three
d2x bases behaved identically; a writable block overlay did not help; NBD
reads completed. `mattrib` confirmed the incompatible attributes. After only
the attribute change, the live export reported `A` without `R` and a verified
control's banner and physical launch passed. The current live `SAVE5G`
container passes WIT verification for both partitions and still reports the
correct archive attribute.

## 4. Files changed

- `scripts/trace-replay/main.go` and tests: opt-in, read-only JSONL block
  replay with exact-length, hash, EOF and latency reporting.
- `server/host-daemon/vdisk/vdisk.go`: coalesced immutable-extent positional
  reads and strict short-read handling.
- `server/host-daemon/vdisk/read_test.go`: short read, EOF, no-progress,
  split-part, sparse-boundary and concurrent-selection/read coverage.
- `server/host-daemon/vdisk/vdisk_benchmark_test.go`: reproducible 1 MiB
  payload benchmark.
- `diagnostics/squeakquel/*`: inventory, media evidence, matrix, isolation,
  root cause, performance and this report.

Pre-existing dirty report/config files were not staged or committed.

## 5. Repair implemented

No new compatibility repair was necessary: the smallest verified correction
is already present in the selected working baseline. No game-ID-specific hack
was added. The new general read loop additionally rejects incomplete backend
reads rather than advancing as though all requested bytes arrived.

## 6. Wii-side settings tested

Historical one-variable tests of d2x-v11-beta3 slots 249/base56,
250/base57 and 251/base58 produced the same failure. No IOS/cIOS, NAND,
global loader setting, save, cheat, region, language or video setting was
changed in this task.

## 7. Performance change

A payload request now spans each contiguous source extent in one
verify/open/pread/close cycle rather than repeating that cycle every 512
bytes. It retains positional I/O, zero-fill, source identity checks and
read-only behavior. No cache, governor, priority, overclock, filesystem or
power-loss setting changed.

## 8. Before/after metrics

Five-run 1 MiB host benchmark mean:

- before: 11.59 ms, 10,240 allocations, about 901 KiB allocated/request;
- after: 29.38 us, 6 allocations, 488 bytes allocated/request;
- result: about 394x lower mapper latency and 99.94% fewer allocations.

These are host mapper metrics. A patched Pi/TrueNAS deployment is required
for Pi CPU, RSS, temperature and throttling data.

After deployment, 200 sequential requests against the live `SAVE5G` extent
completed with no errors in each size class:

- 32 KiB: median 3 us, p95 3,751 us, maximum 6,881 us;
- 64 KiB: median 50 us, p95 3,898 us, maximum 5,369 us.

These timings include the host-side FUSE/libnbd client and can benefit from
page cache; they are not Wii USB latency. Exact WBFS-header, disc-header and
final-sector hashes matched the pre-deployment values. A read after more than
35 idle seconds also succeeded.

## 9. Regression results

- `make test`: pass
- `make server`: pass
- `make static`: pass
- `go test ./scripts/trace-replay`: pass
- `go test -race ./server/host-daemon/vdisk ./tests/unit`: pass
- `deploy/truenas/validate-compose.sh`: pass
- WIT 3.01a verification of live `SAVE5G`: both partitions pass
- live replay: first, disc-header and final sectors pass; deliberate
  beyond-EOF request correctly fails

An unscoped `go test ./...` and `go vet ./...` encounter permission-denied
paths inside retained pi-gen rootfs build artifacts. The project-scoped
Makefile commands avoid those generated roots and passed. Tests were not
weakened.

## 10. Hardware results

The candidate is live on TrueNAS and passed strict health, mutual-TLS NBD,
read-only export, unchanged 10,967,487,488-byte capacity, FAT catalog,
archive-only `SAVE5G` attributes, exact media-sector hashes and idle-read
checks. No new physical Wii, Pi runtime, original-disc or conventional-USB
test was available. Squeakquel, two working controls, ten cold launches and
the 30-minute gameplay soak remain `DEFERRED_HARDWARE_UNAVAILABLE`.
Historical physical success applies only to the documented control title and
must not be represented as a current Squeakquel pass.

## 11. Remaining risks

- The optimization is deployed on TrueNAS but not measured through a Pi Zero.
- No current USB reset/stall, temperature, undervoltage or throttling trace
  exists.
- The exact TrueNAS dataset source path is unavailable without admin access.
- Original-disc, conventional-USB and alternative full-ISO controls remain.
- The existing SD card contains the unmodified known-working firmware, not
  this host-daemon candidate.

## 12. Exact build and deployment command

The candidate was built, published and deployed after operator approval:

```sh
PROJECT_VERSION=0.1.0-rc.1-squeakquel-io \
  ./scripts/build-oci.sh \
  dist/wiibridge-host-0.1.0-rc.1-squeakquel-io.oci
```

OCI manifest identity:
`networkgames-host:0.1.0-rc.1-squeakquel-io@sha256:ded61132346b902b8e74e966bb1a97fe5b428a8ffce68d9f9e050faabaf14bbc`.
The exact manifest was copied to GHCR with digest preservation and an
independent registry inspection returned the same digest:
`ghcr.io/OWNER/networkgames-host:0.1.0-rc.1-squeakquel-io@sha256:ded61132346b902b8e74e966bb1a97fe5b428a8ffce68d9f9e050faabaf14bbc`.
Debian's signed `skopeo` package was installed on the development host for
the OCI transfer and digest verification.

After detaching the Wii/Pi gadget and snapshotting TrueNAS config/data, the
deployment command is:

```sh
NETWORKGAMES_IMAGE='ghcr.io/OWNER/networkgames-host:0.1.0-rc.1-squeakquel-io@sha256:ded61132346b902b8e74e966bb1a97fe5b428a8ffce68d9f9e050faabaf14bbc' \
  docker compose --env-file deploy/truenas/.env \
  -f deploy/truenas/compose.yaml up -d --no-deps --wait wiibridge-host
```

The operator performed the authenticated TrueNAS UI update. Post-deployment
strict health and read-only NBD/media checks passed.

## 13. Exact rollback command

The prior physically verified compatibility image is:
`ghcr.io/OWNER/wiibridge-host:0.1.0-rc.1-wbfsattrfix.1@sha256:37d94d0c3f11ae8c33f96490fe9c0902b6b417907afd56a14d8dd370f4b2fe80`.
With the Wii/Pi gadget detached:

```sh
WIIBRIDGE_IMAGE='ghcr.io/OWNER/wiibridge-host:0.1.0-rc.1-wbfsattrfix.1@sha256:37d94d0c3f11ae8c33f96490fe9c0902b6b417907afd56a14d8dd370f4b2fe80' \
  docker compose --env-file deploy/truenas/.env \
  -f deploy/truenas/compose.yaml up -d --no-deps --wait wiibridge-host
```

Do not reconnect the gadget until strict health, read-only library mount,
read-only NBD, capacity and snapshot identity are verified.

## Commit structure

1. `test: add squeakquel media and block-read diagnostics`
2. No new compatibility commit; confirmed repair is existing commit `74dfb5b`
3. `perf: reduce usb game read latency`
4. `docs: record diagnosis deployment and rollback`

Final status: `LIVE_DEPLOYMENT_VALIDATED_HARDWARE_VALIDATION_REQUIRED`
