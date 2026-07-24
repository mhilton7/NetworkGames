#!/bin/bash
set -euo pipefail

target=${1:?zero-w-armhf, pi4-arm64, or pi5-arm64}
case "$target" in
  zero-w-armhf)
    branch=master
    commit=314262cb286b8f33327a6f0cbabe14c625021ca0
    goarch=arm
    goarm=6
    ;;
  pi4-arm64|pi5-arm64)
    branch=arm64
    commit=ca8aeed0ae300c2a89f55ce9617d5f96a27e99e5
    goarch=arm64
    goarm=
    ;;
  *) echo "unsupported target: $target" >&2; exit 64 ;;
esac
source_root=$(pwd)
tree="build/pi-gen-${target}"
binary="build/pi/${target}/networkgames-pi-controller"
mkdir -p "$(dirname "$binary")"
GOCACHE="${GOCACHE:-/tmp/networkgames-go-cache}" \
GOPATH="${GOPATH:-/tmp/networkgames-gopath}" \
CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" GOARM="$goarm" \
  go build -trimpath -ldflags="-s -w -buildid=" -o "$binary" ./pi/controller
if test -e "$tree"; then
  sudo rm -rf -- "$tree"
fi
git clone --filter=blob:none --branch "$branch" \
  https://github.com/RPi-Distro/pi-gen.git "$tree"
git -C "$tree" checkout --detach "$commit"
# Only the final board-customized stage is a release image.
touch "$tree/stage2/SKIP_IMAGES"
stage="$source_root/pi/packaging/pi-gen/common/stage-networkgames"
sed "s|@NETWORKGAMES_STAGE@|$stage|" \
  "pi/packaging/pi-gen/${target}/config" > "$tree/config"
export NETWORKGAMES_BOARD_TARGET="$target"
export NETWORKGAMES_SOURCE="$source_root"
export PI_GEN_DIR="$source_root/$tree"
log="dist/networkgames-hostbridge-0.1.0-rc.1-${target}.build.log"
mkdir -p dist
if test -s "$log"; then
  mkdir -p "reports/firmware/${target}/rejected-builds"
  cp "$log" "reports/firmware/${target}/rejected-builds/$(date -u +%Y%m%dT%H%M%SZ).log"
fi
(
  cd "$tree"
  sudo --preserve-env=NETWORKGAMES_BOARD_TARGET,NETWORKGAMES_SOURCE,PI_GEN_DIR \
    ./build.sh
) 2>&1 | tee "$source_root/$log"
image=$(find "$tree/deploy" -maxdepth 1 \
  -name "*networkgames-hostbridge-0.1.0-rc.1-${target}.img" -print -quit)
test -n "$image"
"$source_root/scripts/sanitize-firmware-image.sh" "$image"
cp --reflink=auto "$image" "dist/networkgames-hostbridge-0.1.0-rc.1-${target}.img"
