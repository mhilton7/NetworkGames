# Future hardware acceptance plan

All items below currently have status `DEFERRED_HARDWARE_UNAVAILABLE`.

For each Zero W, Pi 4, and Pi 5 independently:

1. Verify first boot, identity generation, wrong-board recovery, network
   provisioning, power budget, thermal behavior, and clean reboot.
2. Validate the documented separate power/data arrangement and every cable.
3. Enumerate the read-only gadget on an isolated Linux USB host; exercise block
   sizes, SCSI write protection, disconnect, packet loss, latency, reset, and
   short/long outage semantics.
4. Confirm snapshot identity remains fixed across reconnect and that a changed
   export is refused.
5. On a Nintendo Wii with USB Loader GX, measure enumeration limits, >4 GiB
   virtual split naming/boundaries, catalog behavior, game launch, random and
   sequential latency, reboot/reconnect behavior, and an extended gameplay soak
   using user-owned legal fixtures.
6. Capture sanitized logs, kernel traces, power measurements, loader version,
   firmware hashes, pass/fail outcomes, and discovered safe timeout policy.

No outage-transparency, Wii, or USB Loader GX claim may be made until this plan
passes on all claimed boards.

For the complete-library profile, additionally verify that one activation shows
every prepared GameCube title, launch at least three titles through Nintendont,
exercise a valid two-disc set, switch Wii→GameCube→Wii ten times, and confirm
the complete Wii snapshot remains unchanged. Record generated image size, Pi
kernel/firmware, loader/runtime versions, cable, power source, and any USB reset
or shutdown. Host-only image inspection is not physical acceptance.
