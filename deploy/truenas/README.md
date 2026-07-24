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
- `/mnt/<POOL>/apps/networkgames/config`: read/write configuration.
- `/mnt/<POOL>/apps/networkgames/data`: read/write compact metadata.
- `/mnt/<POOL>/apps/networkgames/certs`: runtime read-only certificates.
- `/mnt/<POOL>/apps/networkgames/logs`: read/write logs.
- `/mnt/<POOL>/apps/networkgames/backups`: read/write backups.
- UID/GID: a dedicated non-root account (example 568:568), configured rather
  than assumed.
- Management port 8443: bind only to the management LAN address.
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

At upstream and TrueNAS firewalls, allow TCP 8443 only from the management CIDR
and TCP 10809 only from explicit Pi IPs or their dedicated CIDR. Deny both from
WAN networks. Compose port publishing does not replace firewall policy.

Paste the resolved Compose YAML into Install via YAML. After installation,
confirm healthy state, then capture `docker inspect`/Apps details, mounts,
listeners, and sanitized logs using `tests/truenas/capture-evidence.sh`.

Take frequent ZFS snapshots of config and data, daily snapshots of logs if
required, and replicate backups off-system. The source dataset follows its
existing game-library snapshot policy. Removing the app must never select
“delete host datasets.”
