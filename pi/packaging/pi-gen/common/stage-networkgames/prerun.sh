#!/bin/bash
set -euo pipefail

if test ! -d "${ROOTFS_DIR}"; then
  copy_previous
fi
target=${NETWORKGAMES_BOARD_TARGET:?}
install -d -m 0755 \
  "${ROOTFS_DIR}/usr/libexec" \
  "${ROOTFS_DIR}/usr/share/networkgames" \
  "${ROOTFS_DIR}/etc/systemd/system" \
  "${ROOTFS_DIR}/etc/sudoers.d" \
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
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-firstboot.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-firstboot.service"
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-controller.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-controller.service"
install -m 0444 "${NETWORKGAMES_SOURCE}/pi/packaging/systemd/networkgames-recover.service" \
  "${ROOTFS_DIR}/etc/systemd/system/networkgames-recover.service"
install -m 0440 "${NETWORKGAMES_SOURCE}/pi/packaging/pi-gen/common/files/networkgames-sudoers" \
  "${ROOTFS_DIR}/etc/sudoers.d/networkgames"
printf '%s\n' "$target" > "${ROOTFS_DIR}/usr/share/networkgames/board-target"
printf 'VERSION=0.1.0-rc.1\nTARGET=%s\nPI_GEN_COMMIT=%s\n' \
  "$target" "$(git -C "${PI_GEN_DIR}" rev-parse HEAD)" \
  > "${ROOTFS_DIR}/usr/share/networkgames/build-identity"
touch "${ROOTFS_DIR}/boot/firmware/ssh.disabled-by-networkgames"
printf '\ndtoverlay=dwc2,dr_mode=peripheral\n' >> "${ROOTFS_DIR}/boot/firmware/config.txt"
sed -i '1 s/$/ modules-load=dwc2/' "${ROOTFS_DIR}/boot/firmware/cmdline.txt"
ln -sf /etc/systemd/system/networkgames-firstboot.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/networkgames-firstboot.service"
ln -sf /etc/systemd/system/networkgames-controller.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/networkgames-controller.service"
ln -sf /etc/systemd/system/networkgames-recover.service \
  "${ROOTFS_DIR}/etc/systemd/system/multi-user.target.wants/networkgames-recover.service"
rm -f "${ROOTFS_DIR}/etc/machine-id" "${ROOTFS_DIR}"/etc/ssh/ssh_host_*
