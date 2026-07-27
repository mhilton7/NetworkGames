# Pi first run and recovery

Flash the board-specific `.img.xz` after verifying its SHA-256. Never flash a
Pi 4 image to Pi 5 or vice versa. On first boot, unique machine identity, SSH
keys (only if SSH is later enabled), local setup certificate, and administrator
token are generated. The same first boot creates a device-unique WPA2 setup
access point and writes its SSID, Wi-Fi passphrase, management URL, and
management login to `WIIBRIDGE-SETUP.txt` on the boot partition. These
credentials are not shared between images or devices.

The setup access point uses `10.77.0.1/24`; open
`https://10.77.0.1:9443/` while connected to it. A browser warning is expected
for the device-unique self-signed setup certificate. Authenticate as `admin`
with the management password from `WIIBRIDGE-SETUP.txt`, then open
**Network and bridge setup**. The generated management password is 12 lowercase
hexadecimal characters. It is case-sensitive and contains only `0-9` and
`a-f`.

Provision `/etc/wiibridge/bridge.env` with `NBD_HOST`, `NBD_PORT`, export
name, CA, and unique client certificate/key paths. The helper always supplies
`nbd-client -x` and therefore cannot select plaintext. Provision an authorized USB VID/PID
through the service environment; no unlicensed production identity is included.
Ethernet is preferred for Pi 4/5. Zero W provisioning must set the regulatory
country and use 2.4 GHz Wi-Fi. The UI accepts Wi-Fi-only setup when the server
credentials are not ready; after saving, reboot and locate the Pi's DHCP lease
to continue setup on port 9443. If client Wi-Fi cannot connect, the setup access
point returns automatically.

After a client Wi-Fi profile exists, leave all three Wi-Fi fields blank to
retain it while saving server, TLS, or USB settings. Enter country, SSID, and
password together only when changing the network.

The controller refuses gadget attachment until the board matches the image,
the NBD export is connected and read-only, and a production VID/PID is present.
The Pi automatically discovers its USB Device Controller, but it cannot
discover or assign an authorized VID/PID; provision both values explicitly.
The dashboard provides server test/connect and USB attach/detach controls.
Automatic validated connect-and-attach at boot is opt-in, requires both USB
identity values, is disabled during forced recovery, and retries only within a
bounded systemd start-limit window.
Catalog switches require detach, disconnect, identity validation, reconnect,
and reattach.

After Pi provisioning, normal switching, status, reconciliation, reboot, and
shutdown are performed from the Host dashboard. The Pi page remains a local
recovery surface. Host browser bootstrap login is `admin` / `wiibridge` on a
fresh Host data directory and must be replaced at first login; it is separate
from this Pi's management password.

To force recovery mode, create an empty `wiibridge-recovery` file on the
boot partition and reboot. Remove the marker after recovery. Password SSH
remains disabled.
