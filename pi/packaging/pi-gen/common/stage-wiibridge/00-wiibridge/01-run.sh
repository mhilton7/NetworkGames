#!/bin/bash
set -e

on_chroot <<'CHROOT'
getent group wiibridge >/dev/null || groupadd --system wiibridge
id wiibridge >/dev/null 2>&1 || useradd --system --gid wiibridge \
  --home-dir /var/lib/wiibridge --create-home --shell /usr/sbin/nologin wiibridge
install -d -o wiibridge -g wiibridge -m 0750 /run/wiibridge /var/lib/wiibridge
install -d -o root -g systemd-journal -m 2755 /var/log/journal
systemctl disable ssh.service 2>/dev/null || true
systemctl disable hostapd.service 2>/dev/null || true
systemctl mask hostapd.service 2>/dev/null || true
passwd --lock wiibridge-setup
CHROOT
