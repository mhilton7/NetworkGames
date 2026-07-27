# Security model

NBD plaintext export selection is refused until `NBD_OPT_STARTTLS` completes
with TLS 1.3 and a client certificate chained to the configured client CA.
Exports are read-only; write, trim, and write-zero requests return errors.
Source identity is checked before every payload read.

The container is non-root with a read-only root, all capabilities dropped,
no-new-privileges, bounded memory/PIDs/logs/tmpfs, no devices, and no Docker
socket. `/library` and `/certs` are read-only binds.

Each Pi generates machine identity, setup TLS identity, and an administrator
token on first boot. Production client credentials are provisioned uniquely and
are not embedded. SSH password login is disabled. Privileged operations accept
only a fixed action vocabulary and verify board, TLS URI, NBD health, and
read-only state before gadget attachment.
