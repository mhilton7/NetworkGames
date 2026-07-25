#!/bin/bash
set -euo pipefail

shellcheck \
  pi/packaging/pi-gen/common/files/networkgames-helper \
  pi/packaging/pi-gen/common/files/networkgames-gadget \
  pi/packaging/pi-gen/common/files/networkgames-firstboot \
  pi/packaging/pi-gen/common/files/networkgames-setup-ap \
  pi/packaging/pi-gen/common/files/networkgames-provision \
  scripts/repair-live-card.sh
verify_root=build/systemd-verify-root
mkdir -p "$verify_root/etc/systemd/system" "$verify_root/usr/lib/systemd/system" \
  "$verify_root/usr/libexec" "$verify_root/usr/bin" "$verify_root/usr/sbin"
cp -a /usr/lib/systemd/system/. "$verify_root/usr/lib/systemd/system/"
cp pi/packaging/systemd/*.service "$verify_root/etc/systemd/system/"
cp pi/packaging/pi-gen/common/files/networkgames-firstboot \
  pi/packaging/pi-gen/common/files/networkgames-helper \
  pi/packaging/pi-gen/common/files/networkgames-setup-ap \
  pi/packaging/pi-gen/common/files/networkgames-provision \
  "$verify_root/usr/libexec/"
cp build/bin/networkgames-host "$verify_root/usr/bin/networkgames-pi-controller"
for command in hostapd dnsmasq; do
  printf '#!/bin/sh\nexit 0\n' > "$verify_root/usr/sbin/$command"
  chmod 0755 "$verify_root/usr/sbin/$command"
done
systemd-analyze verify --root="$verify_root" \
  networkgames-firstboot.service networkgames-controller.service \
  networkgames-auto-attach.service \
  networkgames-recover.service networkgames-setup-ap.service \
  networkgames-setup-hostapd.service networkgames-setup-dnsmasq.service
for target in zero-w-armhf pi4-arm64 pi5-arm64; do
  test "$(grep -c "^IMG_NAME=.*${target}$" "pi/packaging/pi-gen/${target}/config")" = 1
done
grep -q 'lun.0/ro' pi/packaging/pi-gen/common/files/networkgames-gadget
grep -q 'blockdev --getro' pi/packaging/pi-gen/common/files/networkgames-gadget
grep -q 'nbd-client -x' pi/packaging/pi-gen/common/files/networkgames-helper
if grep -q 'modprobe nbd' pi/packaging/pi-gen/common/files/networkgames-helper; then
  echo "the sandboxed helper must not load kernel modules" >&2
  exit 1
fi
grep -qx 'nbd' \
  pi/packaging/pi-gen/common/files/networkgames-modules-load.conf
grep -qx 'options nbd nbds_max=1' \
  pi/packaging/pi-gen/common/files/networkgames-modprobe.conf
grep -q 'ProtectKernelModules=yes' \
  pi/packaging/systemd/networkgames-controller.service
grep -q 'After=systemd-modules-load.service' \
  pi/packaging/systemd/networkgames-controller.service
grep -q 'auto-attach)' pi/packaging/pi-gen/common/files/networkgames-helper
grep -q 'auto-attach-ready)' pi/packaging/pi-gen/common/files/networkgames-helper
grep -q 'export NETWORKGAMES_USB_VID NETWORKGAMES_USB_PID' \
  pi/packaging/pi-gen/common/files/networkgames-helper
grep -q 'ExecCondition=/usr/libexec/networkgames-helper auto-attach-ready' \
  pi/packaging/systemd/networkgames-auto-attach.service
grep -q 'udevadm settle --timeout=5' \
  pi/packaging/pi-gen/common/files/networkgames-helper
grep -q 'poweroff)' pi/packaging/pi-gen/common/files/networkgames-helper
grep -q 'systemctl poweroff --no-block' \
  pi/packaging/pi-gen/common/files/networkgames-helper
grep -q 'networkgames-helper poweroff' \
  pi/packaging/pi-gen/common/files/networkgames-sudoers
grep -q 'ConditionPathExists=/etc/networkgames/auto-attach' \
  pi/packaging/systemd/networkgames-auto-attach.service
grep -q 'exit 0' pi/packaging/pi-gen/common/files/networkgames-helper
grep -q 'networkgames-setup' pi/packaging/pi-gen/common/files/networkgames-firstboot
grep -Fq "admin_token=\$(openssl rand -hex 6)" \
  pi/packaging/pi-gen/common/files/networkgames-firstboot
grep -q '10.77.0.1/24' pi/packaging/pi-gen/common/files/networkgames-setup-ap
grep -q 'wpa_key_mgmt=WPA-PSK' \
  pi/packaging/pi-gen/common/files/networkgames-firstboot
grep -q 'systemctl start networkgames-setup-hostapd.service' \
  pi/packaging/pi-gen/common/files/networkgames-setup-ap
grep -q 'dhcp-range=10.77.0.20,10.77.0.100' \
  pi/packaging/pi-gen/common/files/networkgames-dnsmasq.conf
grep -q 'dhcp-leasefile=/run/networkgames-dnsmasq/dnsmasq.leases' \
  pi/packaging/pi-gen/common/files/networkgames-dnsmasq.conf
grep -q '^User=dnsmasq$' \
  pi/packaging/systemd/networkgames-setup-dnsmasq.service
grep -q 'conf-file=/etc/networkgames-dnsmasq.conf' \
  pi/packaging/systemd/networkgames-setup-dnsmasq.service
grep -q 'networkgames-client' pi/packaging/pi-gen/common/files/networkgames-provision
grep -q 'wifi-update' pi/packaging/pi-gen/common/files/networkgames-provision
grep -q 'verify -purpose sslclient' \
  pi/packaging/pi-gen/common/files/networkgames-provision
grep -Fq \
  'ReadWritePaths=/run/networkgames /etc/NetworkManager/system-connections /etc/networkgames /boot/firmware' \
  pi/packaging/systemd/networkgames-controller.service
grep -q 'Storage=persistent' \
  pi/packaging/pi-gen/common/files/networkgames-journald.conf
! grep -R -E --include='*' \
  'BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|password[=:][^ ]+|psk[=:][^ ]+' \
  pi deploy server config 2>/dev/null
