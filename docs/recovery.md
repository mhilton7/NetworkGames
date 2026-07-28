# Bridge recovery

If a large library takes time to scan after a Host restart, open the normal
Host HTTPS address. WiiBridge displays `Scanning Wii library`, `Building Wii
virtual disk`, or `Finalizing validated exports` instead of leaving the port
closed. Do not repeatedly restart the container. `/healthz` indicates process
liveness; `/readyz` remains HTTP 503 until the safe Wii export is ready.
GameCube scan and deep validation continue in the background after Wii
readiness, and do not prevent the authenticated dashboard or Wii NBD export.

If TrueNAS still reports the running container as unhealthy, inspect
`docker logs`. The in-container health command now reports missing CA files,
certificate-chain failures, connection errors, and non-200 HTTP responses
instead of exiting silently. Loopback health checks validate the full trusted
server-auth certificate chain without requiring an older certificate to
contain a `127.0.0.1` subjectAltName.

Verify the exact running artifact rather than trusting a mutable release tag:

```sh
sudo docker exec ix-wiibridge-wiibridge-host-1 /wiibridge-host version
sudo docker inspect --format '{{.Config.Image}} image_id={{.Image}} started={{.State.StartedAt}} restarts={{.RestartCount}} oom={{.State.OOMKilled}} health={{.State.Health.Status}}' ix-wiibridge-wiibridge-host-1
sudo docker image inspect --format 'id={{.Id}} revision={{index .Config.Labels "org.opencontainers.image.revision"}} created={{index .Config.Labels "org.opencontainers.image.created"}}' "$(sudo docker inspect --format '{{.Image}}' ix-wiibridge-wiibridge-host-1)"
```

The binary commit and OCI revision label must match the intended immutable
image. These commands do not print environment variables or credentials.
During a delayed phase, use timestamped container logs and the JSON health
endpoints to identify the exact phase before changing limits or timeouts.

On backend loss the bridge must remain detached until the configured export,
snapshot identity, and virtual size validate again. A changed identity is never
accepted as a reconnect. The local recovery service exposes diagnostics and
network reprovisioning but does not attach the USB gadget on a wrong board.

A failed source scan preserves the last complete catalog and marks affected
games offline. Do not acknowledge missing games until the source is available
and two complete scans have confirmed absence. An offline GameCube source
retains its generation, validation receipt, and independent save association.
A changed source revokes the receipt and requires validation/rebuild.

For a blocked emulated card, keep GameCube detached and inspect the Save Overlay
recovery state. Startup replays a complete journal or rolls an interrupted
activation back to `.previous-confirmed.*`. `SAVE-RECOVERY-AMBIGUOUS` means
more than one state could be authoritative: preserve the entire card directory
and use a validated backup restore; do not delete journal or checkpoint files
by hand. Wii and physical-card mode remain independent.

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

For Host browser-password recovery, stop the Host and back up and remove only
`/data/auth/password.json`. Restarting restores the bootstrap login and
mandatory password-change flow. Do not remove the API token, Pi token,
certificates, database, GameCube generations, or save backups.

To roll back the complete-library dashboard, safely detach USB, stop the Host,
and restore the previous container digest or Git commit. The read-only source
library and `/data/gamecube/saves` remain preserved. Configure an older Host
for `physical` because it cannot authorize format-1 save extents. New
`/data/gamecube/library/generations` may be retained or removed while the Host
is stopped; older Host versions ignore it. Wii remains the startup profile.

Before the first schema-2 database transaction, an existing schema-1 database
is checkpointed and copied atomically to
`/data/wiibridge.sqlite3.pre-schema2.bak`; an existing backup is never
overwritten. To roll the database back, keep USB detached, stop the Host, move
the current `wiibridge.sqlite3` and any matching `-wal`/`-shm` files to a
separate retained directory, copy the backup to `wiibridge.sqlite3` with mode
`0600`, and then start the older Host. Do not replace the database while
either Host version is running. The schema-2 save directories and performance
checkpoint are independent and should be retained.

Schema-1 GameCube generations can contain a full `library.img`. They are
reported as legacy and are never deleted at startup. Reclaim their space only
after a schema-2 no-copy generation validates: activate Wii, detach USB,
disconnect NBD, stop the Host, verify the candidate is directly beneath
`/data/gamecube/library/generations`, and remove that one legacy directory
without following symlinks. Preserve `/data/gamecube/saves` and
`/data/auth`.
