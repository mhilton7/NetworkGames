#!/bin/bash
set -e

on_chroot <<'CHROOT'
getent group networkgames >/dev/null || groupadd --system networkgames
id networkgames >/dev/null 2>&1 || useradd --system --gid networkgames \
  --home-dir /var/lib/networkgames --create-home --shell /usr/sbin/nologin networkgames
install -d -o networkgames -g networkgames -m 0750 /run/networkgames /var/lib/networkgames
systemctl disable ssh.service 2>/dev/null || true
passwd --lock networkgames-setup
CHROOT
