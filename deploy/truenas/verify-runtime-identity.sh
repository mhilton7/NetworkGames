#!/bin/sh
set -eu

: "${WIIBRIDGE_EXPECTED_IMAGE:?set the digest-pinned expected image reference}"
: "${WIIBRIDGE_EXPECTED_REVISION:?set the expected 40-character Git revision}"
: "${WIIBRIDGE_CONTAINER:?set the running WiiBridge container name or ID}"
: "${WIIBRIDGE_HOST_URL:?set the deployed HTTPS base URL}"
: "${WIIBRIDGE_CA_FILE:?set the trusted Host CA certificate path}"

expected_capability=${WIIBRIDGE_EXPECTED_CAPABILITY:-wii-fat32-exact-fsinfo-split-v1}
report=${WIIBRIDGE_IDENTITY_REPORT:-wiibridge-runtime-identity.json}

case "$WIIBRIDGE_EXPECTED_IMAGE" in
  *@sha256:*) expected_digest=${WIIBRIDGE_EXPECTED_IMAGE##*@} ;;
  *) echo "WIIBRIDGE_EXPECTED_IMAGE must include @sha256:<digest>" >&2; exit 2 ;;
esac
case "$WIIBRIDGE_EXPECTED_REVISION" in
  *[!0-9a-f]*|'') echo "WIIBRIDGE_EXPECTED_REVISION must be lowercase hexadecimal" >&2; exit 2 ;;
esac
test "${#WIIBRIDGE_EXPECTED_REVISION}" -eq 40 || {
  echo "WIIBRIDGE_EXPECTED_REVISION must contain 40 hexadecimal characters" >&2
  exit 2
}
case "$WIIBRIDGE_HOST_URL" in
  https://*) ;;
  *) echo "WIIBRIDGE_HOST_URL must use HTTPS" >&2; exit 2 ;;
esac
test -r "$WIIBRIDGE_CA_FILE" || {
  echo "trusted CA file is not readable: $WIIBRIDGE_CA_FILE" >&2
  exit 2
}
for command in docker curl jq awk sed mktemp; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "required command unavailable: $command" >&2
    exit 3
  }
done

tmp=$(mktemp -d /tmp/wiibridge-runtime-identity.XXXXXX)
case "$tmp" in /tmp/wiibridge-runtime-identity.*) ;; *) exit 3 ;; esac
cleanup() {
  find "$tmp" -type f -exec shred -u -- {} + 2>/dev/null || true
  find "$tmp" -depth -delete 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM
umask 077

container_id=$(docker inspect --format '{{.Id}}' "$WIIBRIDGE_CONTAINER")
configured_image=$(docker inspect --format '{{.Config.Image}}' "$WIIBRIDGE_CONTAINER")
running_image_id=$(docker inspect --format '{{.Image}}' "$WIIBRIDGE_CONTAINER")
started_at=$(docker inspect --format '{{.State.StartedAt}}' "$WIIBRIDGE_CONTAINER")
restart_count=$(docker inspect --format '{{.RestartCount}}' "$WIIBRIDGE_CONTAINER")
health_state=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}unavailable{{end}}' "$WIIBRIDGE_CONTAINER")
case "$configured_image" in
  *@sha256:*) configured_digest=${configured_image##*@} ;;
  *) echo "configured image is not digest-pinned" >&2; exit 4 ;;
esac
test "$configured_digest" = "$expected_digest" || {
  echo "configured image digest does not match expected digest" >&2
  exit 4
}

repo_digests=$(docker image inspect --format '{{json .RepoDigests}}' "$running_image_id")
printf '%s\n' "$repo_digests" |
  jq -e --arg suffix "@$expected_digest" 'any(.[]; endswith($suffix))' >/dev/null || {
    echo "running image digest does not match expected digest" >&2
    exit 4
  }
oci_revision=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$running_image_id")
oci_created=$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.created"}}' "$running_image_id")
test "$oci_revision" = "$WIIBRIDGE_EXPECTED_REVISION" || {
  echo "OCI revision does not match expected revision" >&2
  exit 4
}

binary_version=$(docker exec "$WIIBRIDGE_CONTAINER" /wiibridge-host version)
binary_revision=$(printf '%s\n' "$binary_version" | sed -n 's/^commit //p')
binary_dirty=$(printf '%s\n' "$binary_version" | sed -n 's/^dirty //p')
test "$binary_revision" = "$WIIBRIDGE_EXPECTED_REVISION" || {
  echo "running binary revision does not match expected revision" >&2
  exit 4
}
test "$binary_revision" = "$oci_revision" || {
  echo "running binary and OCI revisions differ" >&2
  exit 4
}
test "$binary_dirty" = false || {
  echo "running binary is a dirty build" >&2
  exit 4
}

docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' "$WIIBRIDGE_CONTAINER" |
  awk '
    /^WIIBRIDGE_ADMIN_TOKEN=/ {
      sub(/^WIIBRIDGE_ADMIN_TOKEN=/, "")
      gsub(/\\/, "\\\\")
      gsub(/"/, "\\\"")
      print "user = \"admin:" $0 "\""
      found=1
    }
    END { if (!found) exit 1 }
  ' > "$tmp/curl.conf" || {
    echo "running container has no administrator token" >&2
    exit 4
  }
chmod 600 "$tmp/curl.conf"

curl -fsS --cacert "$WIIBRIDGE_CA_FILE" --config "$tmp/curl.conf" \
  "$WIIBRIDGE_HOST_URL/api/v1/compatibility" > "$tmp/compatibility.json"
curl -fsS --cacert "$WIIBRIDGE_CA_FILE" \
  "$WIIBRIDGE_HOST_URL/healthz" > "$tmp/health.json"
curl -fsS --cacert "$WIIBRIDGE_CA_FILE" \
  "$WIIBRIDGE_HOST_URL/readyz" > "$tmp/ready.json"

api_revision=$(jq -r '.host.revision // empty' "$tmp/compatibility.json")
api_dirty=$(jq -r 'if .host | has("buildDirty") then .host.buildDirty else true end' \
  "$tmp/compatibility.json")
test "$api_revision" = "$WIIBRIDGE_EXPECTED_REVISION" || {
  echo "Host API revision does not match expected revision" >&2
  exit 4
}
test "$api_dirty" = false || {
  echo "Host API reports a dirty build" >&2
  exit 4
}
jq -e --arg capability "$expected_capability" \
  '.host.capabilities | index($capability) != null' \
  "$tmp/compatibility.json" >/dev/null || {
    echo "expected filesystem-format capability is missing" >&2
    exit 4
  }
jq -e '.status == "healthy"' "$tmp/health.json" >/dev/null
jq -e '.status == "ready"' "$tmp/ready.json" >/dev/null

jq -n \
  --arg checked_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg container_id "$container_id" \
  --arg configured_image "$configured_image" \
  --arg running_image_id "$running_image_id" \
  --arg digest "$expected_digest" \
  --arg revision "$binary_revision" \
  --arg oci_created "$oci_created" \
  --arg started_at "$started_at" \
  --argjson restart_count "$restart_count" \
  --arg health_state "$health_state" \
  --arg capability "$expected_capability" \
  '{checked_at:$checked_at,status:"PASS",container_id:$container_id,
    configured_image:$configured_image,running_image_id:$running_image_id,
    registry_digest:$digest,revision:$revision,dirty:false,
    oci_created:$oci_created,container_started_at:$started_at,
    restart_count:$restart_count,health_state:$health_state,
    ready:true,required_capability:$capability}' > "$report"
cat "$report"
