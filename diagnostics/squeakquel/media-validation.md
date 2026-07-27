# SAVE5G media validation

Validation was read-only. No source image was renamed, converted, repaired,
or written.

## Exact tested object

- Live virtual path: `/wbfs/SAVE5G.wbfs`
- Container library path: `/library/SAVE5G.wbfs` (the underlying TrueNAS
  dataset path was not available to this host)
- Diagnostic copy:
  `/tmp/wiibridge-squeakquel-live.rFIAep/SAVE5G.wbfs`
- Format: single-file WBFS
- Logical and allocated copy size: 857,735,168 bytes
- SHA-256:
  `2a8bcaf56f02b6b232c650da945495aa07419e9b82d1fd563615f4a608d5970d`
- Header ID: `SAVE5G`
- Region/title reported by WIT: USA, Alvin and the Chipmunks: The Squeakquel
- Source view: FAT32 file over a read-only mutual-TLS NBD export
- FAT attribute: archive `A` (`0x20`), with no DOS read-only `R` (`0x01`)

The similarly named `SA3E5G` image is a different game and historically had
an H0 integrity failure. It was not used as Squeakquel evidence.

## Commands and results

Wiimms ISO Tool 3.01a was installed from the signed Debian repository solely
for diagnostics. `wit list-lll` identified `SAVE5G`. `wit verify -vv` exited
zero and reported both update and encrypted/scrubbed data partitions `+OK`.
The WBFS header, disc header, partition table and allocation map were
therefore readable and internally consistent.

The opt-in replay command checked the WBFS header, disc-header sector and
final sector against precomputed SHA-256 values. Every in-range request
returned exactly 512 bytes. A deliberate 1,024-byte read beginning at the
last sector returned 512 bytes plus `EOF`, proving the harness detects
truncation rather than accepting it.

Representative command:

```sh
go run ./scripts/trace-replay -target /path/to/read-only/SAVE5G.wbfs \
  -trace /path/to/trace.jsonl
```

The complete-file copy through the live NBD/FAT path took 25.367 seconds,
approximately 33.8 MB/s. This is an end-to-end diagnostic copy measurement,
not a Wii USB throughput claim.

## Boundary and parser findings

- The host mapper and NBD path use `int64` file offsets and guarded `uint64`
  request bounds.
- Virtual capacity is derived deterministically from the catalog and remained
  stable throughout this session.
- Unmapped virtual clusters are explicitly zero-filled.
- Source identity (device, inode, size and modification time) is verified
  before payload reads.
- New tests cover short reads, zero-progress reads, EOF, unaligned
  cross-extent reads, split-source boundaries, sparse/zero extents and
  concurrent reads from independent selected disks.
- Existing tests cover invalid disk bounds, first-sector metadata,
  deterministic capacity, large virtual FAT file splitting, mutation
  rejection, FAT fsck, and mutual-TLS read-only NBD.

No local Redump DAT or legal reference ISO hash was available. The WBFS hash
cannot be compared directly to a full-disc ISO hash because WBFS is a
converted/scrubbed container. The passing partition verification is strong
container evidence but does not substitute for an original-disc test.
