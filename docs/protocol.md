# Host and firmware compatibility protocol

Compatibility descriptor schema 1 is returned by the authenticated Pi
management status/compatibility API. It is independent of NBD framing and
product-version strings:

```json
{
  "schemaVersion": 1,
  "protocolMin": 1,
  "protocolMax": 1,
  "productVersion": "<linked build version>",
  "revision": "<linked Git revision>",
  "buildTime": "<linked RFC3339 build time>",
  "buildDirty": false,
  "platform": "host or firmware",
  "board": "<detected Pi board for firmware>",
  "deviceId": "<hashed Pi machine identity>",
  "capabilities": []
}
```

The firmware build links the Makefile version, Git revision, and UTC build
time. Its board is detected at runtime; its stable device ID is a truncated
SHA-256 of machine identity. The Host retains the last result for display and
diagnostics, but performs a fresh pinned-certificate, token-authenticated HTTPS
probe immediately before every state-changing operation.

## Capability identifiers

| Capability | Meaning |
|---|---|
| `wii-read-only-export-v1` | Wii synthetic read-only export |
| `gamecube-schema2-no-copy-v1` | schema-2 mapped GameCube export |
| `gamecube-physical-memory-card-v1` | physical-card mode |
| `gamecube-save-overlay-v1` | bounded emulated save extent transport |
| `safe-platform-switching-v1` | ordered platform transition |
| `usb-detach-v1` / `usb-attach-v1` | typed gadget detach/attach |
| `nbd-connect-v1` / `nbd-disconnect-v1` | typed NBD connect/disconnect |
| `automatic-platform-switching-v1` | full coordinated transition |
| `startup-readiness-v1` | separate startup/readiness reporting |
| `firmware-reboot-v1` / `firmware-shutdown-v1` | typed power operation |
| `diagnostic-status-v1` | authenticated structured status |
| `runtime-metrics-v1` | bounded Pi runtime metrics |
| `source-offline-status-v1` | Host source-availability diagnostics |

Capability names are lowercase, version-suffixed contracts. Unknown advertised
capabilities are ignored. An unknown required capability is missing and blocks
that operation.

## Operation matrix

| Operation | Required firmware capabilities |
|---|---|
| Wii connect | Wii read-only, NBD connect, USB attach |
| GameCube physical | schema 2, physical card, safe switching, NBD connect, USB attach |
| GameCube emulated | schema 2, save overlay, safe switching, NBD connect, USB attach |
| Safe disconnect | USB detach, NBD disconnect |
| Automatic switch | automatic/safe switching, detach, disconnect, connect, attach |
| Reboot / shutdown | corresponding explicit power capability |
| Pi telemetry | runtime metrics is optional |

Evaluation validates schema, protocol-range overlap, descriptors, platform,
board, capabilities, and pinned device identity. States are `compatible`,
`compatible-with-warnings`, `blocked`, `unreachable`, and `unknown`. Missing
optional telemetry degrades only Pi metrics. Missing save-overlay support
blocks only emulated GameCube; safe Wii and physical modes may still pass their
own evaluation.

Stable failures include `COMPAT-DESCRIPTOR-MALFORMED`,
`COMPAT-PROTOCOL-NO-OVERLAP`, `COMPAT-CAPABILITY-MISSING`,
`COMPAT-DEVICE-IDENTITY-MISMATCH`, `COMPAT-FIRMWARE-UNREACHABLE`, and
`COMPAT-OPERATION-BLOCKED`. Responses contain no token, certificate, or key.

`GET /api/v1/compatibility` on the Host returns cached diagnostic state and
marks it stale after 30 seconds. `POST` retries a live status evaluation.
Neither response grants authority for a later action.
