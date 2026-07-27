# Physical GameCube validation

Status: **not executed**. The feature has not been deployed, no Wii SD card
was modified, and no user-owned GameCube image was selected for hardware
testing.

After explicit deployment approval, record each attempt here:

1. Verify rollback host image and Pi image.
2. Cold boot Pi; select host GameCube export while disconnected.
3. On Pi choose the matching physical-card or emulated-card connection mode,
   attach USB, then cold boot Wii.
4. Confirm one correctly identified title in USB Loader GX.
5. Launch through Nintendont and record mount/reset/kernel/application logs.
6. Test single disc, two disc at a legitimate switch point, audio streaming,
   new/existing emulated save, physical card, multiple controllers, and a
   legally available second region.
7. Verify two save generations and restore.
8. Complete ten cold GameCube launches and ten Wii/GameCube switches.
9. Return to Wii mode and launch two known-good Wii games, including `SAVE5G`.
10. Record temperature, throttling, USB reset/stall, latency, and timing data.

No item is marked passed until physically observed.
