# Automatic platform switching

WiiBridge can coordinate the existing Host export manager and Raspberry Pi
controller. This removes the manual detach/disconnect/reconnect sequence while
keeping Wii and GameCube as isolated storage profiles. It does not add a title
changer and does not modify USB Loader GX or Nintendont.

## Safety sequence

Every automated switch is serialized and follows this order:

```text
Pi detach USB
→ Pi disconnect NBD
→ Host waits for active I/O to close
→ Host validates and selects the requested export
→ Pi reconnects NBD in the matching read/write mode
→ Pi validates that mode
→ Pi attaches USB
```

The Host permits only `detach`, `disconnect`, `connect-wii`,
`connect-gamecube-physical`, `connect-gamecube-emulated`, and `attach`. It
cannot invoke poweroff, provisioning, cache deletion, or arbitrary commands.

If preparation or disconnection fails, the Host export is not changed. If
reconnection or attachment fails, WiiBridge performs a best-effort detach and
disconnect and reports that USB remains detached. It never switches the backing
store while the Wii is connected.

## Enable it

Automatic switching is deliberately opt-in. On the Pi, identify its management
URL and copy only its public management certificate:

```sh
sudo sha256sum /etc/wiibridge/device.crt
sudo cp /etc/wiibridge/device.crt /path/to/removable-or-secure-transfer/pi-device.crt
```

Do not copy `device.key`. Place `pi-device.crt` in the TrueNAS certificate
dataset and compare its SHA-256 with the value recorded on the Pi.

Set all three values in `deploy/truenas/.env`:

```text
WIIBRIDGE_PI_URL=https://PI_MANAGEMENT_ADDRESS:9443
WIIBRIDGE_PI_ADMIN_TOKEN=THE_EXISTING_PI_MANAGEMENT_PASSWORD
WIIBRIDGE_PI_CERT=/certs/pi-device.crt
```

The token is the Pi management password created during first boot, not the Host
administrator token. Keep it out of Git. Restrict the Host app's network access
to the single Pi address and TCP 9443.

Restart only the Host app after reviewing the configuration. The dashboard
will show “Automatic safety sequence enabled.” If any of the three values is
missing or invalid, the Host refuses startup instead of silently falling back
to an insecure partial configuration.

## Operation

Prepare GameCube exports before switching. Then select a ready GameCube entry
or click **Use Wii export**. The dashboard reports completion only after the Pi
has reattached USB. USB Loader GX may need its normal device refresh after USB
re-enumeration.

All Wii titles remain together in the normal Wii catalog. This feature does not
switch individual Wii titles. Current GameCube cache behavior is unchanged.

## Disable or recover

To return to manual operation, clear all three `WIIBRIDGE_PI_*` values and
restart the Host app.

After a reported failure, inspect the Pi dashboard on port 9443. Confirm USB is
detached before using its fixed recovery actions. Wii remains the startup and
recovery default after an ordinary reboot.

The Pi management certificate is pinned exactly. When that certificate is
rotated or the Pi is reprovisioned, copy and verify the new public certificate
before updating `WIIBRIDGE_PI_CERT`. A certificate mismatch stops automation;
it never falls back to unauthenticated TLS.
