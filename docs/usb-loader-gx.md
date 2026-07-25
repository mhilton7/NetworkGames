# USB Loader GX future configuration

The bridge presents one read-only MBR/FAT32 LUN containing `/wbfs`. After the
future Linux USB-host gate passes, configure USB Loader GX to use the enumerated
USB device and do not enable game installation, deletion, or rename operations.
Catalog switching requires USB detach, NBD disconnect, selection of a new
immutable snapshot, full validation, reconnect, and reattach.

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

No Nintendo Wii or USB Loader GX test was performed. Detection, enumeration,
virtual `.wbfs`/`.wbf1` split compatibility, game launch, and gameplay soak are
all `DEFERRED_HARDWARE_UNAVAILABLE`; see `hardware-acceptance-plan.md`.
