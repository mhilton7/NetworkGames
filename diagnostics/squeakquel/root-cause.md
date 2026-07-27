# Root-cause determination

## Exact failure

Historically, `SAVE5G` was detected by USB Loader GX r1283 but its animated
banner was blank. Launch produced a brief transition and returned to the Wii
System Menu before gameplay. The same symptom affected verified controls.

## Confirmed cause

The synthesized FAT32 directory marked every WBFS file DOS read-only
(`0x01`). USB Loader GX enumerates headers through a read-only path, but its
banner and boot code reopens split/WBFS files with `O_RDWR`. FAT therefore
allowed enumeration and rejected the later open before payload execution.

Evidence chain:

1. `SAVE5G` passed WIT partition verification and occupied the low-LBA range.
2. Verified high-LBA controls failed identically.
3. Three installed d2x bases failed identically.
4. NBD requests completed; a temporary writable block overlay did not help.
5. Independent `mattrib` showed read-only attributes on every virtual WBFS.
6. Commit `74dfb5bb2d66c2a558c9de7690ee89c0c31daa70` changed only the
   synthesized file attribute to archive (`0x20`), leaving NBD and gadget
   read-only.
7. The deployed replacement showed `A` without `R`; a previously blank
   control banner then displayed and the game launched on the physical Wii.
8. The present live `SAVE5G` entry also reports `A` without `R`.

The smallest verified compatibility repair is therefore already an ancestor
of this branch. Reimplementing it or adding a `SAVE5G` special case would be
unnecessary and risk divergence. For that reason there is no new
`fix: correct confirmed squeakquel launch failure` commit in this task.

## Additional read-path correction

Audit found that the current payload path advanced by the requested length
after a source `ReadAt` returned a short read with `EOF`. The optimized path
now uses a full positional-read loop and returns EOF/no-progress errors rather
than treating missing bytes as success. This is a general transport
correctness guard, not evidence that the validated Squeakquel image was
truncated.

No region, video, cIOS, per-title metadata, USB VID/PID, certificate, Wi-Fi,
filesystem-protection or shutdown setting was changed.
