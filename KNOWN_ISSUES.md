# Known issues

## USB Loader GX still freezes on the valid 32 KiB large-volume export

The 32 KiB geometry repair is deployed and its 1.65 TiB-class export is valid
FAT32. Independent reads pass, but USB Loader GX r1283 still freezes at
`Initializing USB devices`. During the physical failure the Pi, NBD session,
and USB gadget remain healthy and error-free, and completed NBD requests do
not advance.

The live FSInfo sector reports `0xffffffff` for its free-cluster count. USB
Loader GX's bundled custom libfat 1.1.5 interprets that unknown sentinel by
walking the complete FAT during its mount path. The source follow-up reserves
one real free cluster and publishes exact nonzero FSInfo values, avoiding that
capacity-proportional mount scan. Tests and a local OCI build pass. A clean
image must be published and deployed on TrueNAS before the next physical Wii
retest. The current local OCI was built from a dirty worktree and must not be
treated as a release artifact.

Status: `PENDING` — exact-FSInfo replacement publication and deployment
required.
