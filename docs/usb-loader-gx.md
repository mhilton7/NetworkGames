# USB Loader GX configuration and regression diagnosis

The bridge presents one read-only MBR/FAT32 LUN containing `/wbfs`. After the
future Linux USB-host gate passes, configure USB Loader GX to use the enumerated
USB device and do not enable game installation, deletion, or rename operations.
Catalog switching requires USB detach, NBD disconnect, selection of a new
immutable snapshot, full validation, reconnect, and reattach.

For the large Wii profile, verify the exact live disk rather than source code
alone:

- 32 KiB FAT32 clusters.
- Valid and matching primary/backup boot and FSInfo sectors.
- Exact nonzero FSInfo free count and genuinely unallocated next-free cluster.
- Matching FAT copies and valid directory chains.
- Archive attribute `0x20` on every `.wbfs`/`.wbfN` entry.
- DOS read-only, hidden, and system attributes clear.
- Each full virtual split is `4 GiB - 32 KiB`, matching r1283's one-cluster-
  below-4-GiB boundary.

Do not substitute a FAT read-only attribute for block-level protection. r1283
opens existing WBFS split parts with `O_RDWR` for banner and boot paths; the
directory entry must permit that open while NBD and the USB LUN reject writes.

`Initializing USB devices` covers storage discovery and FAT mounting. Unknown
FSInfo values make r1283's bundled libfat walk the complete FAT. `Loading
resources` can remain visible while the loader enumerates `/wbfs`; compare NBD
counters before calling it a dead transport. If the traversal finishes and
requests stop, inspect split metadata, caches, and guest-side handoff rather
than weakening TLS or adding network caching.

On a Pi Zero W, use the micro-USB port labeled **USB**, not **PWR IN**, for the
data connection. Use a known data-capable cable and validate enumeration on a
separate Linux USB host before connecting a Wii. Avoid an unsafe dual-power
arrangement; confirm the intended power path for the cable and host setup.

The USB Device Controller is discovered automatically. USB VID/PID ownership is
not discoverable from a host and no arbitrary production identity is bundled.
Provision an authorized pair, test the NBD server, select **Connect server**,
and then select **Attach USB**. The dashboard's USB link state becomes
`configured` after a host successfully enumerates the gadget. The opt-in
automatic mode performs the same validated test/connect/attach sequence after
boot and remains off during forced recovery.

Physical Wii testing confirmed that deploying exact FSInfo advances r1283 past
`Initializing USB devices`. A subsequent complete-catalog traversal finished
without NBD, source, or USB errors but did not reach the GUI. The live export
then exposed 59 `4 GiB - 4 KiB` segments whose 32 KiB-cluster chains span
exactly 2^32 bytes; the local `4 GiB - 32 KiB` correction passes independent
FAT and source-byte regressions but is not yet deployed or physically accepted.
Animated banners, sustained gameplay, and reconnect acceptance therefore
remain `PENDING`, not passed; see `hardware-acceptance-plan.md`.
