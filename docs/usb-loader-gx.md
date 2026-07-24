# USB Loader GX future configuration

The bridge presents one read-only MBR/FAT32 LUN containing `/wbfs`. After the
future Linux USB-host gate passes, configure USB Loader GX to use the enumerated
USB device and do not enable game installation, deletion, or rename operations.
Catalog switching requires USB detach, NBD disconnect, selection of a new
immutable snapshot, full validation, reconnect, and reattach.

No Nintendo Wii or USB Loader GX test was performed. Detection, enumeration,
virtual `.wbfs`/`.wbf1` split compatibility, game launch, and gameplay soak are
all `DEFERRED_HARDWARE_UNAVAILABLE`; see `hardware-acceptance-plan.md`.
