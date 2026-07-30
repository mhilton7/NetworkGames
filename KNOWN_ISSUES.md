# Known issues

## Corrected large Wii Host image is not deployed

The live TrueNAS Host still synthesizes the 1.65 TiB Wii export with fixed
4 KiB FAT32 clusters. That layout exceeds FAT32's usable cluster-number range,
so USB Loader GX can freeze while initializing the USB device even though the
Pi, NBD session, and USB gadget all report healthy.

The source repair and a locally verified OCI image are available in this
worktree. A clean image must be published and deployed on TrueNAS before the
physical Wii retest. The current local OCI was built from a dirty worktree and
must not be treated as a release artifact.

Status: `PENDING` — external TrueNAS deployment access is unavailable.
