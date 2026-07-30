# Known issues

## USB Loader GX still freezes on the valid 8 KiB large-volume export

The first geometry repair is deployed and its 1.65 TiB-class export is valid
FAT32 with 8 KiB clusters. Independent reads pass, but USB Loader GX r1283
still freezes at `Initializing USB devices`. During the physical failure the
Pi, NBD session, and USB gadget remain healthy and error-free, and completed
NBD requests do not advance.

The follow-up source policy uses the conventional 32 KiB cluster geometry for
Wii libraries at or above 32 GiB, reducing the live-scale FAT from roughly
844 MiB to 211 MiB and the data-cluster count from roughly 221 million to
55.3 million. Tests and a local OCI build pass. A clean image must be
published and deployed on TrueNAS before the next physical Wii retest. The
current local OCI was built from a dirty worktree and must not be treated as a
release artifact.

Status: `PENDING` — 32 KiB replacement publication and deployment required.
