# Known issues and external facts

- No connected TrueNAS Community Edition target is visible from this build
  environment. Exact version, Apps backend compatibility, datasets, ACLs, and
  port conflicts cannot be truthfully live-verified here.
- Docker Engine and Compose were installed on the build host and the hardened
  container and Compose parser gates now pass.
- NBD is pinned to mutual-X.509 TLS 1.2 for interoperability with the release
  host's libnbd 1.22/GnuTLS 3.8.9 client. That client fails to select its
  certificate for the equivalent Go TLS 1.3 request; HTTPS remains TLS 1.3.
- No physical Raspberry Pi, USB host, Wii, USB Loader GX installation, safe
  microSD target, cable/power arrangement, production VID/PID, or legal private
  game fixture is available. These are non-blocking and all corresponding gates
  are `DEFERRED_HARDWARE_UNAVAILABLE`.
- The required filename `CODEX_PRODUCTION_PROMPT.txt` was absent; the complete
  controlling content was present and read from
  `networkgames_wbfs_hostbridge_truenas_3pi_no_hardware_codex_prompt.txt`.
- The firmware images were each built from a clean board work tree and passed
  offline validation, but a second complete build was not run to establish
  byte-for-byte reproducibility. The corresponding release gate remains
  `PENDING`; no reproducibility claim is made.
- Release provenance identifies the committed base and a source-tree digest,
  and explicitly records `sourceWorktreeDirty: true` because this implementation
  has not been committed.
- GitHub publication is currently blocked by an invalid GitHub CLI token for
  the active `mhilton7` account. Run `gh auth login -h github.com` before
  retrying the publisher; no release upload has been recorded as passed.
