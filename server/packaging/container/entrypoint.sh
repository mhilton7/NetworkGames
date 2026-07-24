#!/bin/sh
set -eu

: "${NETWORKGAMES_UID:?NETWORKGAMES_UID is required}"
: "${NETWORKGAMES_GID:?NETWORKGAMES_GID is required}"
test "$(id -u)" = "$NETWORKGAMES_UID"
test "$(id -g)" = "$NETWORKGAMES_GID"
exec /networkgames-host
