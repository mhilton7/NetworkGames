# Wii compatibility test matrix

This separates historical physical evidence from tests performed in the
current task. The Wii and Pi were not connected during the current host-side
work, so no new physical result is marked passed.

## Historical reproduction before the general repair

| Variable | Result |
|---|---|
| Loader | USB Loader GX r1283 |
| Console | Physical Wii |
| Title | `SAVE5G`, verified low-LBA Squeakquel WBFS |
| Symptom | Title enumerated; animated banner remained blank; launch briefly transitioned and returned to the Wii System Menu |
| Offset control | Same result below 2 GiB, eliminating high-LBA/32-bit position as the differentiator |
| IOS249, d2x-v11-beta3 base 56 | Same failure |
| IOS250, d2x-v11-beta3 base 57 | Same failure |
| IOS251, d2x-v11-beta3 base 58 | Same failure |
| Alternate DOL | Built-in lookup resolved off; no title entry |
| Transport state | Pi remained NBD-connected and USB-attached; traced NBD reads completed |

The historical notes do not record Wiimote connection state or Reset/Power
responsiveness for the Squeakquel attempt, so those observations are unknown,
not inferred.

## Controls

Before the repair, verified 10 Minute Solution and American Mensa Academy
showed the same blank-banner/return-to-menu symptom. A writable block-overlay
comparison accepted host writes but still failed, which rejected SCSI write
protection as the cause.

After changing only the synthesized WBFS file attribute, 10 Minute Solution's
banner displayed and the game launched on the physical Wii. This is the
available post-repair control. The repository does not contain evidence that
Squeakquel itself or a second independent title was physically retested after
that deployment.

## Required current physical matrix

All rows below remain `DEFERRED_HARDWARE_UNAVAILABLE`:

1. Defaults, optional patches/cheats/EmuNAND disabled, Disc Default video.
2. `SAVE5G` on automatic game IOS.
3. IOS249/base56, only if the installed SysCheck still confirms that base.
4. IOS250/base57, one-variable comparison only.
5. Disc Default versus NTSC-U video only if a black-screen symptom appears.
6. One small working control and one streaming-heavy working control.
7. Ten cold launches, one 30-minute gameplay run, return/relaunch.

Do not change global loader defaults or install cIOS. Because the identified
failure was shared across titles and the live FAT attribute is already
correct, no `SAVE5G`-specific compatibility profile is justified.
