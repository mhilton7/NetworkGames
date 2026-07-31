# Known issues

## USB Loader GX stalls after large-catalog enumeration

The exact-FSInfo repair is deployed and USB Loader GX now advances beyond
`Initializing USB devices`, confirming that the prior mount blocker is
resolved.

The former `Loading resources` stall covered a finite, error-free traversal;
Pi requests stopped after the loader encountered incompatible full-size WBFS
segments.

A read-only audit of the exact live export found 59 virtual segments sized
4,294,963,200 bytes (`4 GiB - 4 KiB`). On the deployed 32 KiB-cluster volume,
each segment's FAT chain spans exactly 4,294,967,296 bytes. That value wraps to
zero in 32-bit FAT chain-length accounting. Independent `fsck.fat -n`
reproduces the zero-length-chain failure on the live export and on a compact
synthetic fixture. USB Loader GX r1283 instead uses 4,294,934,528-byte
(`4 GiB - 32 KiB`) splits, explicitly keeping each part one cluster below the
boundary.

The deployed builder now matches the loader boundary. The unchanged regression passes
independent `fsck.fat`, archive-only attribute checks, unique alias and LFN
checks, segment ordering, and exact banner-range/split-boundary source reads.
full validation and the regenerated live disk passes independent `fsck.fat`.
The physical loader reaches the complete catalog and allows selection, so this
specific post-enumeration blocker is corrected.

The Wii SD filesystem was dirty and its FAT copies differed. A full
allocated-file recovery archive plus raw MBR/boot/FAT metadata were retained
before repairing it. The loader caches were structurally valid, backed up, and
moved aside. A Wii-only, no-cache, disc-title, list-mode configuration is now
staged, with free-space and banner-cache work disabled.

Banner and game behavior is not fully corrected. `SM2E52` displayed its banner
but failed launch; verified low-LBA `R22E01` loaded once, returned through the
configured Return To USB Loader path, then froze on its next selection. During
that repeat freeze, Pi requests stayed at 5,787 with zero failures, reconnects,
or USB resets and idle CPU. The configured gadget was not receiving useful Wii
commands. `SLREWR` showed the same no-read selection freeze.

Status: `PENDING` — isolate Return-To-Loader state with a physical Wii Reset
and cold Loader relaunch while leaving the corrected Host and Pi unchanged.
