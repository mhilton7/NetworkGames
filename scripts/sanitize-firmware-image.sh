#!/bin/bash
set -euo pipefail

image=${1:?image path}
test -f "$image"
partition_text=$(/usr/sbin/sfdisk -d "$image")
root_start=$(printf '%s\n' "$partition_text" |
  awk -F'[=, ]+' '/img2/{for(i=1;i<=NF;i++)if($i=="start"){print $(i+1);exit}}')
test -n "$root_start"
tmp=$(mktemp -d)
loop=
cleanup() {
  sudo umount "$tmp" 2>/dev/null || true
  test -z "$loop" || sudo losetup -d "$loop" 2>/dev/null || true
  rmdir "$tmp" 2>/dev/null || true
}
trap cleanup EXIT
loop=$(sudo losetup --find --show --partscan "$image")
sudo mount "${loop}p2" "$tmp"
sudo sh -c ": > '$tmp/etc/machine-id'"
sudo find "$tmp/etc/ssh" -maxdepth 1 -type f -name 'ssh_host_*' -delete
sudo umount "$tmp"
sudo losetup -d "$loop"
loop=
