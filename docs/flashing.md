# Firmware flashing

Select the artifact whose target exactly matches the board. Verify both the
`.img.xz.sha256` file and the release provenance before writing it. Use
Raspberry Pi Imager's custom-image option, or:

```sh
xz -dc networkgames-hostbridge-VERSION-TARGET.img.xz |
  sudo dd of=/dev/EXPLICIT_MICROSD_DEVICE bs=4M oflag=direct status=progress
sync
```

The destination must be an explicitly inspected removable microSD device.
Never substitute a guessed device path. Pi 4 and Pi 5 images are deliberately
separate. First boot creates machine identity and SSH host keys; client TLS
credentials are provisioned uniquely and are not embedded in the image.

Physical flashing and boot remain `DEFERRED_HARDWARE_UNAVAILABLE` for this
release candidate.
