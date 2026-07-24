# Pi first run and recovery

Flash the board-specific `.img.xz` after verifying its SHA-256. Never flash a
Pi 4 image to Pi 5 or vice versa. On first boot, unique machine identity, SSH
keys (only if SSH is later enabled), local setup certificate, and administrator
token are generated.

Provision `/etc/networkgames/bridge.env` with `NBD_HOST`, `NBD_PORT`, export
name, CA, and unique client certificate/key paths. The helper always supplies
`nbd-client -x` and therefore cannot select plaintext. Provision an authorized USB VID/PID
through the service environment; no unlicensed production identity is included.
Ethernet is preferred for Pi 4/5. Zero W provisioning must set the regulatory
country and use 2.4 GHz Wi-Fi.

The controller refuses gadget attachment until the board matches the image,
the NBD export is connected and read-only, and a production VID/PID is present.
Catalog switches require detach, disconnect, identity validation, reconnect,
and reattach.
