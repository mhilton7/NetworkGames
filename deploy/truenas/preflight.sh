#!/bin/sh
set -eu

report=${1:-preflight-report.json}
version=UNKNOWN
backend=UNSUPPORTED_OR_UNKNOWN
arch=$(uname -m)
if command -v midclt >/dev/null 2>&1; then
  version=$(midclt call system.version | tr -d '"')
  if midclt call app.query >/dev/null 2>&1; then
    backend=COMPOSE_CUSTOM_APPS_CANDIDATE
  fi
elif test -r /etc/version; then
  version=$(sed -n '1p' /etc/version)
fi
case "$arch" in
  x86_64) image_arch=amd64 ;;
  aarch64) image_arch=arm64 ;;
  *) image_arch=UNSUPPORTED ;;
esac
for variable in NETWORKGAMES_LIBRARY_PATH NETWORKGAMES_CONFIG_PATH \
  NETWORKGAMES_DATA_PATH NETWORKGAMES_CERTS_PATH NETWORKGAMES_LOGS_PATH \
  NETWORKGAMES_BACKUPS_PATH NETWORKGAMES_HTTPS_BIND NETWORKGAMES_NBD_BIND; do
  value=$(printenv "$variable" || true)
  test -n "$value" || {
    echo "$variable is required" >&2
    exit 2
  }
done
for directory in "$NETWORKGAMES_LIBRARY_PATH" "$NETWORKGAMES_CONFIG_PATH" \
  "$NETWORKGAMES_DATA_PATH" "$NETWORKGAMES_CERTS_PATH" \
  "$NETWORKGAMES_LOGS_PATH" "$NETWORKGAMES_BACKUPS_PATH"; do
  test -d "$directory" || {
    echo "missing directory: $directory" >&2
    exit 2
  }
done
library_mode=$(findmnt -no OPTIONS -T "$NETWORKGAMES_LIBRARY_PATH" 2>/dev/null || echo UNKNOWN)
jq -n \
  --arg version "$version" --arg backend "$backend" --arg arch "$arch" \
  --arg image_arch "$image_arch" --arg library_mode "$library_mode" \
  '{truenas_version:$version,apps_backend:$backend,host_architecture:$arch,
    required_image_architecture:$image_arch,library_mount_options:$library_mode,
    compatible:($backend=="COMPOSE_CUSTOM_APPS_CANDIDATE" and $image_arch!="UNSUPPORTED")}' \
  > "$report"
cat "$report"
test "$backend" = COMPOSE_CUSTOM_APPS_CANDIDATE
