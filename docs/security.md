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

Browser authentication is independent from API and Pi credentials. A fresh
data directory accepts the documented bootstrap login `admin` / `wiibridge`
and forces replacement before administrative mutations. Changed passwords are
salted Argon2id records under `/data/auth`; plaintext is never persisted.
Browser sessions exist only in Host memory and use Secure, HttpOnly,
SameSite=Strict cookies and per-session CSRF tokens. API Bearer and Basic
compatibility continues to use `WIIBRIDGE_ADMIN_TOKEN`. Pi management continues
to use its separate token and exact certificate pin.

The compiled web bundle uses only same-origin templates, CSS, and JavaScript.
The Content Security Policy forbids external assets and inline script/style.
Browser controls cannot provide shell commands, paths, NBD URLs, systemd units,
or privileged helper action names.
