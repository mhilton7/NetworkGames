#!/bin/bash
set -euo pipefail

if test ! -d "${ROOTFS_DIR}"; then
  copy_previous
fi
target=${NETWORKGAMES_BOARD_TARGET:?}
install -d -m 0755 \
  "${ROOTFS_DIR}/usr/libexec" \
  "${ROOTFS_DIR}/usr/share/networkgames" \
  "${ROOTFS_DIR}/etc/networkgames" \
  "${ROOTFS_DIR}/etc/modules-load.d" \
  "${ROOTFS_DIR}/etc/modprobe.d" \
  "${ROOTFS_DIR}/etc/systemd/system" \
  "${ROOTFS_DIR}/etc/systemd/journald.conf.d" \
  "${ROOTFS_DIR}/etc/sudoers.d" \
  "${ROOTFS_DIR}/usr/lib/tmpfiles.d" \
  "${ROOTFS_DIR}/var/cache/networkgames" \
  "${ROOTFS_DIR}/run/networkgames"
install -m 0555 "${NETWORKGAMES_SOURCE}/build/pi/${target}/networkgames-pi-controller" \
  "${ROOTFS_DIR}/usr/bin/networkgames-pi-controller"
install -m 0555 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-helper" \
  "${ROOTFS_DIR}/usr/libexec/networkgames-helper"
install -m 0555 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-gadget" \
  "${ROOTFS_DIR}/usr/libexec/networkgames-gadget"
install -m 0555 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-firstboot" \
  "${ROOTFS_DIR}/usr/libexec/networkgames-firstboot"
install -m 0555 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-setup-ap" \
  "${ROOTFS_DIR}/usr/libexec/networkgames-setup-ap"
install -m 0555 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-provision" \
  "${ROOTFS_DIR}/usr/libexec/networkgames-provision"
install -m 0644 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-dnsmasq.conf" \
  "${ROOTFS_DIR}/etc/networkgames-dnsmasq.conf"
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-firstboot.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-firstboot.service"
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-controller.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-controller.service"
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-auto-attach.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-auto-attach.service"
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-recover.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-recover.service"
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-setup-ap.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-setup-ap.service"
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-setup-hostapd.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-setup-hostapd.service"
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-setup-dnsmasq.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-setup-dnsmasq.service"
install -m 0440 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-sudoers" \
  "${ROOTFS_DIR}/etc/sudoers.d/networkgames"
install -m 0644 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-journald.conf" \
  "${ROOTFS_DIR}/etc/systemd/journald.conf.d/networkgames.conf"
install -m 0644 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-tmpfiles.conf" \
  "${ROOTFS_DIR}/usr/lib/tmpfiles.d/networkgames.conf"
install -m 0644 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-modules-load.conf" \
  "${ROOTFS_DIR}/etc/modules-load.d/networkgames.conf"
install -m 0644 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-modprobe.conf" \
  "${ROOTFS_DIR}/etc/modprobe.d/networkgames-nbd.conf"
printf '%s\n' "$target" > "${ROOTFS_DIR}/usr/share/networkgames/board-target"
printf 'VERSION=0.1.0-rc.1\nTARGET=%s\nPI_GEN_COMMIT=%s\n' \
  "$target" "$(git -C "${PI_GEN_DIR}" rev-parse HEAD)" \
  > "${ROOTFS_DIR}/usr/share/networkgames/build-identity"
touch "${ROOTFS_DIR}/boot/firmware/ssh.disabled-by-networkgames"
printf 'US\n' > "${ROOTFS_DIR}/boot/firmware/networkgames-country"
printf '\ndtoverlay=dwc2,dr_mode=peripheral\n' >> "${ROOTFS_DIR}/boot/firmware/config.txt"
sed -i '1 s/$/ modules-load=dwc2/' "${ROOTFS_DIR}/boot/firmware/cmdline.txt"
ln -sf /etc/systemd/system/networkgames-firstboot.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/networkgames-firstboot.service"
ln -sf /etc/systemd/system/networkgames-controller.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/networkgames-controller.service"
ln -sf /etc/systemd/system/networkgames-auto-attach.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/networkgames-auto-attach.service"
ln -sf /etc/systemd/system/networkgames-recover.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/networkgames-recover.service"
ln -sf /etc/systemd/system/networkgames-setup-ap.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/networkgames-setup-ap.service"
rm -f "${ROOTFS_DIR}/etc/machine-id" "${ROOTFS_DIR}"/etc/ssh/ssh_host_*
