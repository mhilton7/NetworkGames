#!/bin/bash
set -euo pipefail

if test ! -d "${ROOTFS_DIR}"; then
  copy_previous
fi
target=${WIIBRIDGE_BOARD_TARGET:?}
install -d -m 0755 \
  "${ROOTFS_DIR}/usr/libexec" \
  "${ROOTFS_DIR}/usr/share/wiibridge" \
  "${ROOTFS_DIR}/etc/wiibridge" \
  "${ROOTFS_DIR}/etc/modules-load.d" \
  "${ROOTFS_DIR}/etc/modprobe.d" \
  "${ROOTFS_DIR}/etc/systemd/system" \
  "${ROOTFS_DIR}/etc/systemd/journald.conf.d" \
  "${ROOTFS_DIR}/etc/sudoers.d" \
  "${ROOTFS_DIR}/usr/lib/tmpfiles.d" \
  "${ROOTFS_DIR}/var/cache/wiibridge" \
  "${ROOTFS_DIR}/run/wiibridge"
install -m 0555 "${WIIBRIDGE_SOURCE}/build/pi/${target}/wiibridge-pi-controller" \
  "${ROOTFS_DIR}/usr/bin/wiibridge-pi-controller"
install -m 0555 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-helper" \
  "${ROOTFS_DIR}/usr/libexec/wiibridge-helper"
install -m 0555 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-gadget" \
  "${ROOTFS_DIR}/usr/libexec/wiibridge-gadget"
install -m 0555 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-firstboot" \
  "${ROOTFS_DIR}/usr/libexec/wiibridge-firstboot"
install -m 0555 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-setup-ap" \
  "${ROOTFS_DIR}/usr/libexec/wiibridge-setup-ap"
install -m 0555 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-provision" \
  "${ROOTFS_DIR}/usr/libexec/wiibridge-provision"
install -m 0644 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-dnsmasq.conf" \
  "${ROOTFS_DIR}/etc/wiibridge-dnsmasq.conf"
install -m 0444 "${WIIBRIDGE_SOURCE}/pi/packaging/systemd/wiibridge-firstboot.service" \
  "${ROOTFS_DIR}/etc/systemd/system/wiibridge-firstboot.service"
install -m 0444 "${WIIBRIDGE_SOURCE}/pi/packaging/systemd/wiibridge-controller.service" \
  "${ROOTFS_DIR}/etc/systemd/system/wiibridge-controller.service"
install -m 0444 "${WIIBRIDGE_SOURCE}/pi/packaging/systemd/wiibridge-auto-attach.service" \
  "${ROOTFS_DIR}/etc/systemd/system/wiibridge-auto-attach.service"
install -m 0444 "${WIIBRIDGE_SOURCE}/pi/packaging/systemd/wiibridge-recover.service" \
  "${ROOTFS_DIR}/etc/systemd/system/wiibridge-recover.service"
install -m 0444 "${WIIBRIDGE_SOURCE}/pi/packaging/systemd/wiibridge-setup-ap.service" \
  "${ROOTFS_DIR}/etc/systemd/system/wiibridge-setup-ap.service"
install -m 0444 "${WIIBRIDGE_SOURCE}/pi/packaging/systemd/wiibridge-setup-hostapd.service" \
  "${ROOTFS_DIR}/etc/systemd/system/wiibridge-setup-hostapd.service"
install -m 0444 "${WIIBRIDGE_SOURCE}/pi/packaging/systemd/wiibridge-setup-dnsmasq.service" \
  "${ROOTFS_DIR}/etc/systemd/system/wiibridge-setup-dnsmasq.service"
install -m 0440 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-sudoers" \
  "${ROOTFS_DIR}/etc/sudoers.d/wiibridge"
install -m 0644 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-journald.conf" \
  "${ROOTFS_DIR}/etc/systemd/journald.conf.d/wiibridge.conf"
install -m 0644 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-tmpfiles.conf" \
  "${ROOTFS_DIR}/usr/lib/tmpfiles.d/wiibridge.conf"
install -m 0644 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-modules-load.conf" \
  "${ROOTFS_DIR}/etc/modules-load.d/wiibridge.conf"
install -m 0644 "${WIIBRIDGE_SOURCE}/pi/packaging/pi-gen/common/files/wiibridge-modprobe.conf" \
  "${ROOTFS_DIR}/etc/modprobe.d/wiibridge-nbd.conf"
printf '%s\n' "$target" > "${ROOTFS_DIR}/usr/share/wiibridge/board-target"
printf 'VERSION=0.1.0-rc.1\nTARGET=%s\nPI_GEN_COMMIT=%s\n' \
  "$target" "$(git -C "${PI_GEN_DIR}" rev-parse HEAD)" \
  > "${ROOTFS_DIR}/usr/share/wiibridge/build-identity"
touch "${ROOTFS_DIR}/boot/firmware/ssh.disabled-by-wiibridge"
printf 'US\n' > "${ROOTFS_DIR}/boot/firmware/wiibridge-country"
printf '\ndtoverlay=dwc2,dr_mode=peripheral\n' >> "${ROOTFS_DIR}/boot/firmware/config.txt"
sed -i '1 s/$/ modules-load=dwc2/' "${ROOTFS_DIR}/boot/firmware/cmdline.txt"
ln -sf /etc/systemd/system/wiibridge-firstboot.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/wiibridge-firstboot.service"
ln -sf /etc/systemd/system/wiibridge-controller.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/wiibridge-controller.service"
ln -sf /etc/systemd/system/wiibridge-auto-attach.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/wiibridge-auto-attach.service"
ln -sf /etc/systemd/system/wiibridge-recover.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/wiibridge-recover.service"
ln -sf /etc/systemd/system/wiibridge-setup-ap.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/wiibridge-setup-ap.service"
rm -f "${ROOTFS_DIR}/etc/machine-id" "${ROOTFS_DIR}"/etc/ssh/ssh_host_*
