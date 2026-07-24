# Codex project instructions

- The controlling specification is
  `networkgames_wbfs_hostbridge_truenas_3pi_no_hardware_codex_prompt.txt`.
- Build a network-backed, read-only USB mass-storage appliance without a payload
  mirror or payload-bearing raw image.
- Never download or commit copyrighted content; tests use synthetic WBFS data.
- Never record an unexecuted test as passed.
- Record unavailable physical tests as `DEFERRED_HARDWARE_UNAVAILABLE`.
- Preserve progress in `BUILD_STATUS.json`, `WORKLOG.md`,
  `RELEASE_GATES.md`, and `KNOWN_ISSUES.md`.
- Do not expose credentials in logs, arguments, URLs, or reports.
- The required final label is
  `SOFTWARE-COMPLETE RELEASE CANDIDATE — HARDWARE UNVERIFIED`.
