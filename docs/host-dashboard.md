# Host dashboard

The Host dashboard is the normal WiiBridge control center. On a fresh data
directory, sign in over HTTPS with:

```text
Username: admin
Password: wiibridge
```

The first login is restricted to the password-change page. WiiBridge stores
only a salted Argon2id record in `/data/auth/password.json` (directory mode
`0700`, file mode `0600`) and invalidates every browser session after a change.
Sessions are server-side and use a Secure, HttpOnly, SameSite=Strict cookie.
Existing Bearer and Basic API authentication continues to use
`WIIBRIDGE_ADMIN_TOKEN`; it is never placed in a browser cookie.

The two primary controls are:

```text
Activate Wii Library
Activate GameCube Library
```

The Wii control always selects the complete proven Wii snapshot. The GameCube
control selects the current complete validated generation and is disabled until
one exists. Building and updating are separate background operations; opening
the page never starts a build.

GameCube builds report titles, discs, mapped files, metadata generation, and
validation rather than payload-copy bytes. The generated FAT32 disk reads game
payloads through to `/library`; `/data` contains compact metadata only. The
dashboard reports **Legacy copied GameCube generation detected** when an older
schema-1 `library.img` remains, but it does not delete it automatically.

The Pi panel uses the existing pinned management certificate and independent Pi
token. It exposes only typed operations: detach USB, disconnect NBD, connect the
current trusted profile, attach USB, reconcile, reboot, and power off. It does
not expose a shell, arbitrary helper actions, provisioning, firmware flashing,
private keys, or filesystem access.

## Password recovery

1. Safely detach USB and stop the Host container.
2. Back up `/data/auth/password.json`.
3. Remove only `/data/auth/password.json`.
4. Restart the Host.
5. Sign in with `admin` / `wiibridge` and immediately replace it.

This does not alter API/Pi tokens, certificates, games, generated metadata, or
save backups.
