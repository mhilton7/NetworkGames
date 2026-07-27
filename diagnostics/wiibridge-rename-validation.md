# WiiBridge rename validation

Date: 2026-07-26
Branch: `refactor/wiibridge-rename`

## Renamed surfaces

- Project text: `WiiBridge`
- Go module and runtime identifiers: `wiibridge`
- Environment prefix: `WIIBRIDGE_`
- GHCR image: `ghcr.io/mhilton7/wiibridge-host`
- Host binary: `wiibridge-host`
- Pi controller/helper/provisioning files and systemd services
- Pi runtime, state, configuration, cache, and log paths
- ConfigFS gadget and FAT volume labels
- Firmware and release artifact prefixes
- GitHub Actions workflow and publishing defaults
- Documentation, schemas, tests, metrics, and synthetic fixtures

## Intentional historical or identity exceptions

- Signed/generated reports under `reports/firmware` retain the names of the
  artifacts they actually describe.
- Baseline, rollback, and repair reports retain old immutable image references
  and backup paths so recovery commands remain valid.
- The USB serial remains `NG-<machine-id>` to preserve the identity of the
  already deployed physical device. Manufacturer and configuration strings
  now say WiiBridge.
- Existing Nintendont and USB Loader GX names are upstream product names.

## Validation

Passed:

```text
go test -race (all source package sets)
make test
make static
make compose
make oci
GOOS=linux GOARCH=arm GOARM=6 go build ./pi/controller
git diff --check
```

Artifacts:

```text
wiibridge-host:0.1.0-rc.1@sha256:9a10a60ac9e1ba63362928ced29c921ccf93ab22545b30c53eda28c35b50220a
host binary SHA-256: 60ce0400b1892becac312d43ed33a6e4cb36e7fb7f3ec46eeb682e8c131f36d6
Pi Zero ARMv6 controller SHA-256: b326c9d490820c1e3c338ff791fbc63180be2e5b4c89417b0cababf4ada393fe
```

The independent Dockerfile build could not contact the local Docker daemon
because this account lacks socket permission. The repository's deterministic
OCI builder and Compose parser both passed, so no elevated daemon access was
requested.

No GitHub push, repository-setting change, deployment, firmware flash, or
hardware action was performed.
