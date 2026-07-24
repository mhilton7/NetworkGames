#!/bin/bash
set -euo pipefail

shellcheck \
  pi/packaging/pi-gen/common/files/networkgames-helper \
  pi/packaging/pi-gen/common/files/networkgames-gadget \
  pi/packaging/pi-gen/common/files/networkgames-firstboot
verify_root=build/systemd-verify-root
mkdir -p "$verify_root/etc/systemd/system" "$verify_root/usr/lib/systemd/system" \
  "$verify_root/usr/libexec" "$verify_root/usr/bin"
cp -a /usr/lib/systemd/system/. "$verify_root/usr/lib/systemd/system/"
cp pi/packaging/systemd/*.service "$verify_root/etc/systemd/system/"
cp pi/packaging/pi-gen/common/files/networkgames-firstboot \
  pi/packaging/pi-gen/common/files/networkgames-helper "$verify_root/usr/libexec/"
cp build/bin/networkgames-host "$verify_root/usr/bin/networkgames-pi-controller"
systemd-analyze verify --root="$verify_root" \
  networkgames-firstboot.service networkgames-controller.service \
  networkgames-recover.service
for target in zero-w-armhf pi4-arm64 pi5-arm64; do
  test "$(grep -c "^IMG_NAME=.*${target}$" "pi/packaging/pi-gen/${target}/config")" = 1
done
grep -q 'lun.0/ro' pi/packaging/pi-gen/common/files/networkgames-gadget
grep -q 'blockdev --getro' pi/packaging/pi-gen/common/files/networkgames-gadget
grep -q 'nbd-client -x' pi/packaging/pi-gen/common/files/networkgames-helper
! grep -R -E --include='*' \
  'BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|password[=:][^ ]+|psk[=:][^ ]+' \
  pi deploy server config 2>/dev/null
