# TrueNAS Community Edition deployment

This app is installed through **Apps → Discover Apps → Install via YAML**. It
does not modify the TrueNAS base OS and must not be granted privileged mode,
devices, host networking, or the Docker socket.

## Compatibility and required values

Run `preflight.sh` in a supported TrueNAS shell before installation. It records
the exact version, architecture, Apps API result, paths, and mount facts. A
result other than `COMPOSE_CUSTOM_APPS_CANDIDATE` must be treated as
incompatible until an administrator confirms that Install via YAML accepts a
Compose model. The normal server image is `linux/amd64`.

Replace these placeholders:

- `/mnt/<POOL>/<WII_DATASET>`: existing library, mounted `/library:ro`.
- `/mnt/<POOL>/apps/wiibridge/config`: read/write configuration.
- `/mnt/<POOL>/apps/wiibridge/data`: read/write compact metadata.
- `/mnt/<POOL>/apps/wiibridge/certs`: runtime read-only certificates.
- `/mnt/<POOL>/apps/wiibridge/logs`: read/write logs.
- `/mnt/<POOL>/apps/wiibridge/backups`: read/write backups.
- UID/GID: a dedicated non-root account (example 568:568), configured rather
  than assumed.
- Management port 8445: bind only to the management LAN address.
- NBD/TLS port 10809: bind only to the Pi-facing trusted address.

Create each child dataset in the TrueNAS UI. Grant the configured UID/GID
read/execute on every parent, read/execute only on the source and certificate
datasets, and modify permissions on config, data, logs, and backups. Do not use
an ACL entry that grants the app account source-file write, delete-child,
write-attributes, write-ACL, or ownership rights. Preserve ACL inheritance for
new metadata files.

Generate a random administrator bearer token of at least 20 characters and
provision a private CA, server certificate, and a unique client certificate per
Pi. Files expected in the certificate dataset are `ca.crt`, `server.crt`,
`server.key`, and `clients-ca.crt`. Never paste private keys into Compose.
The server certificate must cover the address used by Pi clients and
`127.0.0.1` for the container health check; `scripts/tls-provision.sh`
generates both identities.

At upstream and TrueNAS firewalls, allow TCP 8445 only from the management CIDR
and TCP 10809 only from explicit Pi IPs or their dedicated CIDR. Deny both from
WAN networks. Compose port publishing does not replace firewall policy.

Automatic platform switching is optional. To enable it, copy the Pi's exact
`/etc/wiibridge/device.crt` public certificate into the host certificate
dataset, then configure `WIIBRIDGE_PI_ADMIN_TOKEN` and `WIIBRIDGE_PI_CERT`.
`WIIBRIDGE_PI_URL` may provide the initial address, or an administrator can
save a literal Pi IP from the dashboard. HTTPS, fixed port 9443, and certificate
pinning remain mandatory. Do not expose the Pi controller to the internet.
Leave all three values empty to retain manual switching. See
`docs/automatic-platform-switching.md` for certificate-pinning, failure, and
rotation behavior.

Paste the resolved Compose YAML into Install via YAML. After installation,
confirm healthy state, then capture `docker inspect`/Apps details, mounts,
listeners, and sanitized logs using `tests/truenas/capture-evidence.sh`.

The Host image reports its source identity with `/wiibridge-host version`.
Compare that commit with both the expected source revision and the OCI
`org.opencontainers.image.revision` label. A release tag alone is insufficient;
pin the registry digest. `/healthz` is the container liveness check and returns
HTTP 200 while scans are in progress. `/readyz` is deliberately not used by the
container health check because it remains HTTP 503 until the Wii export is
safe. Increasing `start_period` cannot correct a listener that was opened too
late.

The supplied Compose configuration limits the container to 512 MiB and sets
Go's managed-heap target to 384 MiB. The Wii FAT is generated sector-by-sector
from compact cluster-chain descriptors, so its RAM use no longer grows with
the apparent FAT size of a multi-terabyte library. Do not remove these limits
to mask a startup problem. If a deployment has a measured need for more
headroom, raise `WIIBRIDGE_GO_MEMORY_LIMIT` and `WIIBRIDGE_MEMORY_LIMIT`
together while leaving at least 128 MiB between the Go target and container
limit for stacks, binaries, TLS, and kernel-backed buffers.

Take frequent ZFS snapshots of config and data, daily snapshots of logs if
required, and replicate backups off-system. The source dataset follows its
existing game-library snapshot policy. Removing the app must never select
“delete host datasets.”
