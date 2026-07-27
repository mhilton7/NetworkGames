#!/bin/sh
set -eu

: "${WIIBRIDGE_UID:?WIIBRIDGE_UID is required}"
: "${WIIBRIDGE_GID:?WIIBRIDGE_GID is required}"
test "$(id -u)" = "$WIIBRIDGE_UID"
test "$(id -g)" = "$WIIBRIDGE_GID"
exec /wiibridge-host
