# Read-path performance

## Method

The benchmark is `BenchmarkPayloadRead1MiB` in
`server/host-daemon/vdisk/vdisk_benchmark_test.go`. It builds a legal
synthetic WBFS fixture, selects its payload extent, reads 1 MiB through
`vdisk.Disk.ReadAt`, and reports allocations. Both versions ran five times on
the same Ryzen 9 9950X3D host with the same Go 1.25.12 toolchain and warm
filesystem page cache.

This isolates mapper overhead. It is not a claim about Raspberry Pi Zero,
Wi-Fi, SD, USB or Wii throughput.

## Results

| Metric | Before | After | Change |
|---|---:|---:|---:|
| Mean 1 MiB latency across five runs | 11.59 ms | 29.38 us | 394x lower |
| Allocations per request | 10,240 | 6 | 99.94% lower |
| Allocated bytes per request | about 901,133 | 488 | 99.95% lower |

Baseline samples ranged from 11.53 to 11.66 ms. After samples ranged from
29.08 to 29.73 us. No measured mapper metric regressed.

The live pre-change read-only copy of the complete 857,735,168-byte
Squeakquel file took 25.367 seconds (about 33.8 MB/s) through TLS NBD and the
synthesized FAT view.

After approved deployment, 200 live extent reads at each request size
completed without error. For 32 KiB reads, median/p95/maximum were
3/3,751/6,881 us. For 64 KiB reads they were 50/3,898/5,369 us. The live
client used FUSE/libnbd and a warm page cache, so these figures establish
error-free post-deployment latency rather than a directly comparable Pi USB
speedup.

## Change

Previously, a 1 MiB request was split at every 512-byte sector. Each payload
sector repeated source identity verification, open, positional read and
close—up to 2,048 cycles.

The new loop:

- coalesces reads through the remaining contiguous immutable extent;
- keeps positional reads, so no shared seek state is introduced;
- verifies source identity before access;
- opens the source once per request/extent and closes it deterministically;
- retries valid short reads and rejects EOF/no-progress;
- preserves explicit zero-fill and metadata-sector behavior.

No cache, read-ahead, `O_DIRECT`, mmap, real-time priority, governor,
overclock, kernel change, service suppression, `nofua`, filesystem safety
change, or additional persistent state was introduced.

## Commands

```sh
go test -run '^$' -bench BenchmarkPayloadRead1MiB -benchmem -count=5 \
  ./server/host-daemon/vdisk
go test -race ./server/host-daemon/vdisk ./tests/unit
```

Pi CPU, RSS, temperature, throttling, USB resets and end-to-end p50/p95/max
latency remain hardware-deployment measurements. They must be captured before
accepting the optimization for production.
