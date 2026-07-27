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

For platform transitions, the Host permits only `detach`, `disconnect`, `connect-wii`,
`connect-gamecube-physical`, `connect-gamecube-emulated`, and `attach`. It
cannot invoke provisioning, cache deletion, or arbitrary commands. Separate,
explicitly confirmed power controls may invoke only the typed `reboot` and
`poweroff` helpers.

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

## Pi power controls

When automatic coordination is configured, the Host dashboard displays a
collapsed **Pi power controls** panel. Reboot and shutdown each require their
own confirmation checkbox and an authenticated CSRF-protected POST request.
Both actions run the Pi's fixed helper, which:

1. detaches the USB gadget;
2. disconnects NBD;
3. calls `sync`;
4. requests either `systemctl reboot --no-block` or
   `systemctl poweroff --no-block`.

The API equivalents are:

```text
POST /api/v1/pi/reboot    confirm=reboot
POST /api/v1/pi/shutdown  confirm=shutdown
```

These controls require a Pi firmware/controller package containing the typed
`reboot` helper and its exact sudoers rule. Shutdown uses the existing typed
poweroff helper. No arbitrary action name or command text is accepted.

## Live Pi status

When automatic coordination is configured, the Host dashboard requests
`GET /api/v1/pi/status` every three seconds. The Host retrieves each fresh
result through the same certificate-pinned HTTPS connection and independent Pi
administrator token used for switching. The browser never receives that token
or the pinned certificate.

The panel reports controller and provisioning state, export mode, NBD
connection, USB attachment and controller state, detected board, network
addresses, and automatic-attach state. If the Pi is offline, authentication
fails, its certificate does not match, or the request times out, the panel
changes to `Unavailable` and retries. The dashboard receives a deliberately
generic error instead of transport or credential details.

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
