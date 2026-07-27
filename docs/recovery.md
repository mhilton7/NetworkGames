# Bridge recovery

On backend loss the bridge must remain detached until the configured export,
snapshot identity, and virtual size validate again. A changed identity is never
accepted as a reconnect. The local recovery service exposes diagnostics and
network reprovisioning but does not attach the USB gadget on a wrong board.

If normal Wi-Fi is unavailable, the bridge automatically restores its
device-unique setup access point. To force that behavior, shut the Pi down,
insert the microSD card in a trusted computer, create an empty
`wiibridge-recovery` file on the boot partition, and boot again. Read
`WIIBRIDGE-SETUP.txt` from that partition for the WPA2 and HTTPS management
credentials. Remove the marker after recovery.

For operator recovery:

1. Open `https://10.77.0.1:9443/` on the recovery access point, or use the
   bridge's DHCP address on its configured network.
2. Detach USB through the local UI.
3. Correct network, CA, client certificate, server name, export, or catalog.
4. Run the connection and sample-read checks.
5. Confirm the snapshot ID and size match the intended catalog.
6. Reattach only after the controller reports healthy.

Before removing the microSD card, use **Safely power off Pi** on the
authenticated dashboard and wait for the activity LED to stop. Unplugging
power while the filesystems are active can truncate recently written
configuration or credential files.

Do not clear identity or reuse a client private key between devices. Sanitized
diagnostics must omit admin tokens and all private-key material.
