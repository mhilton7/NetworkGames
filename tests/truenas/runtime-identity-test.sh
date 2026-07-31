#!/bin/sh
set -eu

repo=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
tmp=$(mktemp -d /tmp/wiibridge-runtime-identity-test.XXXXXX)
case "$tmp" in /tmp/wiibridge-runtime-identity-test.*) ;; *) exit 91 ;; esac
cleanup() { find "$tmp" -depth -delete 2>/dev/null || true; }
trap cleanup EXIT HUP INT TERM
mkdir -p "$tmp/bin"

cat > "$tmp/bin/docker" <<'EOF'
#!/bin/sh
set -eu
digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
revision=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
if test "$1" = inspect && test "$2" = --format; then
  case "$3" in
    '{{.Id}}') echo container-id ;;
    '{{.Config.Image}}') echo "ghcr.io/example/wiibridge-host@$digest" ;;
    '{{.Image}}') echo image-id ;;
    '{{.State.StartedAt}}') echo 2026-07-31T20:00:00Z ;;
    '{{.RestartCount}}') echo 0 ;;
    '{{if .State.Health}}{{.State.Health.Status}}{{else}}unavailable{{end}}') echo healthy ;;
    '{{range .Config.Env}}{{println .}}{{end}}') echo WIIBRIDGE_ADMIN_TOKEN=synthetic-token-not-a-secret ;;
    *) echo "unexpected inspect format: $3" >&2; exit 2 ;;
  esac
elif test "$1" = image && test "$2" = inspect && test "$3" = --format; then
  case "$4" in
    '{{json .RepoDigests}}') echo "[\"ghcr.io/example/wiibridge-host@$digest\"]" ;;
    '{{index .Config.Labels "org.opencontainers.image.revision"}}') echo "$revision" ;;
    '{{index .Config.Labels "org.opencontainers.image.created"}}') echo 2026-07-31T19:59:00Z ;;
    *) echo "unexpected image format: $4" >&2; exit 2 ;;
  esac
elif test "$1" = exec; then
  cat <<VERSION
WiiBridge 0.1.0-rc.1
commit $revision
built 2026-07-31T19:59:00Z
dirty false
go go1.25.12
target linux/amd64
VERSION
else
  echo "unexpected docker invocation: $*" >&2
  exit 2
fi
EOF

cat > "$tmp/bin/curl" <<'EOF'
#!/bin/sh
set -eu
for argument in "$@"; do url=$argument; done
case "$url" in
  */api/v1/compatibility)
    if test "${MOCK_MISSING_CAPABILITY:-0}" = 1; then capabilities='["wii-read-only-export-v1"]';
    else capabilities='["wii-fat32-exact-fsinfo-split-v1","wii-read-only-export-v1"]'; fi
    printf '{"host":{"revision":"%s","buildDirty":false,"capabilities":%s}}\n' \
      bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb "$capabilities"
    ;;
  */healthz) printf '%s\n' '{"status":"healthy"}' ;;
  */readyz) printf '%s\n' '{"status":"ready"}' ;;
  *) echo "unexpected URL: $url" >&2; exit 2 ;;
esac
EOF
chmod 0755 "$tmp/bin/docker" "$tmp/bin/curl"
: > "$tmp/ca.crt"

expected_image=ghcr.io/example/wiibridge-host@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
expected_revision=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
PATH="$tmp/bin:$PATH" \
WIIBRIDGE_EXPECTED_IMAGE="$expected_image" \
WIIBRIDGE_EXPECTED_REVISION="$expected_revision" \
WIIBRIDGE_CONTAINER=wiibridge-host \
WIIBRIDGE_HOST_URL=https://192.0.2.10:8445 \
WIIBRIDGE_CA_FILE="$tmp/ca.crt" \
WIIBRIDGE_IDENTITY_REPORT="$tmp/pass.json" \
  "$repo/deploy/truenas/verify-runtime-identity.sh" >/dev/null
jq -e '.status == "PASS" and .dirty == false and .ready == true' "$tmp/pass.json" >/dev/null

if PATH="$tmp/bin:$PATH" \
  WIIBRIDGE_EXPECTED_IMAGE=ghcr.io/example/wiibridge-host@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc \
  WIIBRIDGE_EXPECTED_REVISION="$expected_revision" \
  WIIBRIDGE_CONTAINER=wiibridge-host \
  WIIBRIDGE_HOST_URL=https://192.0.2.10:8445 \
  WIIBRIDGE_CA_FILE="$tmp/ca.crt" \
  WIIBRIDGE_IDENTITY_REPORT="$tmp/digest-mismatch.json" \
    "$repo/deploy/truenas/verify-runtime-identity.sh" >/dev/null 2>&1; then
  echo "digest mismatch was accepted" >&2
  exit 1
fi

if PATH="$tmp/bin:$PATH" MOCK_MISSING_CAPABILITY=1 \
  WIIBRIDGE_EXPECTED_IMAGE="$expected_image" \
  WIIBRIDGE_EXPECTED_REVISION="$expected_revision" \
  WIIBRIDGE_CONTAINER=wiibridge-host \
  WIIBRIDGE_HOST_URL=https://192.0.2.10:8445 \
  WIIBRIDGE_CA_FILE="$tmp/ca.crt" \
  WIIBRIDGE_IDENTITY_REPORT="$tmp/missing-capability.json" \
    "$repo/deploy/truenas/verify-runtime-identity.sh" >/dev/null 2>&1; then
  echo "missing filesystem capability was accepted" >&2
  exit 1
fi

echo "runtime identity deployment check: PASS"
