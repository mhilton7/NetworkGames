#!/bin/bash
set -euo pipefail

if test "$#" -lt 3 || test "$#" -gt 5; then
  echo "usage: $0 ROOT_MOUNT BOOT_MOUNT CONTROLLER_BINARY [WIFI_COUNTRY] [--reset-admin-password]" >&2
  exit 64
fi
if test "$(id -u)" -ne 0; then
  echo "repair-live-card must run as root" >&2
  exit 77
fi

root=${1%/}
boot=${2%/}
controller=$3
country=${4:-US}
reset_admin=${5:-}
case "$country" in
  [A-Z][A-Z]) ;;
  *) echo "Wi-Fi country must be a two-letter uppercase code" >&2; exit 64 ;;
esac
case "$reset_admin" in
  ""|--reset-admin-password) ;;
  *) echo "unknown repair option: $reset_admin" >&2; exit 64 ;;
esac
test -n "$root" -a "$root" != /
test -n "$boot" -a "$boot" != /
test -d "$root/etc" -a -d "$boot"
test -x "$controller"
test "$(cat "$root/usr/share/networkgames/board-target")" = zero-w-armhf
test -s "$root/etc/networkgames/admin.token"
test -s "$root/etc/networkgames/device.crt"
test -s "$root/etc/networkgames/device.key"
test -x "$root/usr/sbin/hostapd"
test -x "$root/usr/sbin/dnsmasq"

project=$(cd "$(dirname "$0")/.." && pwd)
files=$project/pi/packaging/pi-gen/common/files
units=$project/pi/packaging/systemd
networkgames_gid=$(
  awk -F: '$1=="networkgames"{print $3; exit}' "$root/etc/group"
)
journal_gid=$(
  awk -F: '$1=="systemd-journal"{print $3; exit}' "$root/etc/group"
)
test -n "$networkgames_gid"
test -n "$journal_gid"

install -o 0 -g 0 -m 0555 "$controller" \
  "$root/usr/bin/networkgames-pi-controller"
install -o 0 -g 0 -m 0555 \
  "$files/networkgames-helper" \
  "$files/networkgames-firstboot" \
  "$files/networkgames-gadget" \
  "$files/networkgames-setup-ap" \
  "$files/networkgames-provision" \
  "$root/usr/libexec/"
install -o 0 -g "$networkgames_gid" -m 0440 \
  "$files/networkgames-sudoers" \
  "$root/etc/sudoers.d/networkgames"
install -o 0 -g 0 -m 0444 \
  "$units/networkgames-firstboot.service" \
  "$units/networkgames-controller.service" \
  "$units/networkgames-auto-attach.service" \
  "$units/networkgames-recover.service" \
  "$units/networkgames-setup-ap.service" \
  "$units/networkgames-setup-hostapd.service" \
  "$units/networkgames-setup-dnsmasq.service" \
  "$root/etc/systemd/system/"
install -d -o 0 -g 0 -m 0755 \
  "$root/etc/systemd/journald.conf.d" \
  "$root/etc/modules-load.d" \
  "$root/etc/modprobe.d" \
  "$root/usr/lib/tmpfiles.d"
install -d -o 0 -g 0 -m 0700 \
  "$root/etc/NetworkManager/system-connections"
install -o 0 -g 0 -m 0644 "$files/networkgames-journald.conf" \
  "$root/etc/systemd/journald.conf.d/networkgames.conf"
install -o 0 -g 0 -m 0644 "$files/networkgames-tmpfiles.conf" \
  "$root/usr/lib/tmpfiles.d/networkgames.conf"
install -o 0 -g 0 -m 0644 "$files/networkgames-modules-load.conf" \
  "$root/etc/modules-load.d/networkgames.conf"
install -o 0 -g 0 -m 0644 "$files/networkgames-modprobe.conf" \
  "$root/etc/modprobe.d/networkgames-nbd.conf"
install -o 0 -g 0 -m 0644 \
  "$files/networkgames-dnsmasq.conf" \
  "$root/etc/networkgames-dnsmasq.conf"
if test -e "$root/etc/networkgames/dnsmasq.conf"; then
  unlink "$root/etc/networkgames/dnsmasq.conf"
fi
install -d -o 0 -g "$journal_gid" -m 2755 "$root/var/log/journal"

ln -sfn /etc/systemd/system/networkgames-controller.service \
  "$root/etc/systemd/system/multi-user.target.wants/networkgames-controller.service"
ln -sfn /etc/systemd/system/networkgames-auto-attach.service \
  "$root/etc/systemd/system/multi-user.target.wants/networkgames-auto-attach.service"
ln -sfn /etc/systemd/system/networkgames-recover.service \
  "$root/etc/systemd/system/multi-user.target.wants/networkgames-recover.service"
ln -sfn /etc/systemd/system/networkgames-setup-ap.service \
  "$root/etc/systemd/system/multi-user.target.wants/networkgames-setup-ap.service"
if test -L "$root/etc/systemd/system/multi-user.target.wants/hostapd.service"; then
  unlink "$root/etc/systemd/system/multi-user.target.wants/hostapd.service"
fi
ln -sfn /dev/null "$root/etc/systemd/system/hostapd.service"

chown 0:"$networkgames_gid" "$root/etc/networkgames"
chmod 0750 "$root/etc/networkgames"
for credential in admin.token device.crt device.key; do
  chown 0:"$networkgames_gid" "$root/etc/networkgames/$credential"
  chmod 0640 "$root/etc/networkgames/$credential"
done
if test -e "$root/etc/networkgames/auto-attach" &&
  ! grep -Eq '^NETWORKGAMES_USB_VID=0x[0-9A-Fa-f]{4}$' \
    "$root/etc/networkgames/bridge.env" ||
  test -e "$root/etc/networkgames/auto-attach" &&
  ! grep -Eq '^NETWORKGAMES_USB_PID=0x[0-9A-Fa-f]{4}$' \
    "$root/etc/networkgames/bridge.env"; then
  unlink "$root/etc/networkgames/auto-attach"
fi
if test "$reset_admin" = --reset-admin-password; then
  admin_tmp=$(mktemp "$root/etc/networkgames/.admin-token.XXXXXX")
  openssl rand -hex 6 > "$admin_tmp"
  chown 0:"$networkgames_gid" "$admin_tmp"
  chmod 0640 "$admin_tmp"
  mv -f "$admin_tmp" "$root/etc/networkgames/admin.token"
fi

printf '%s\n' "$country" > "$boot/networkgames-country"
machine=$(tr -d '[:space:]' < "$root/etc/machine-id")
test "${#machine}" -ge 8
suffix=${machine:0:8}
setup_ssid=NetworkGames-"$suffix"
setup_profile=$root/etc/NetworkManager/system-connections/networkgames-setup.nmconnection
hostapd_config=$root/etc/networkgames/hostapd.conf
if test -s "$hostapd_config"; then
  setup_ssid=$(sed -n 's/^ssid=//p' "$hostapd_config" | head -n 1)
  setup_passphrase=$(sed -n 's/^wpa_passphrase=//p' "$hostapd_config" | head -n 1)
elif test -s "$setup_profile"; then
  setup_ssid=$(sed -n 's/^ssid=//p' "$setup_profile" | head -n 1)
  setup_passphrase=$(sed -n 's/^psk=//p' "$setup_profile" | head -n 1)
else
  setup_passphrase=$(openssl rand -hex 16)
fi
test -n "$setup_ssid"
test -n "$setup_passphrase"
hostapd_tmp=$(mktemp "$root/etc/networkgames/.hostapd.XXXXXX")
{
  printf '%s\n' \
    'interface=wlan0' \
    'driver=nl80211' \
    "ssid=$setup_ssid" \
    "country_code=$country" \
    'ieee80211d=1' \
    'hw_mode=g' \
    'channel=6' \
    'wmm_enabled=0' \
    'auth_algs=1' \
    'wpa=2' \
    'wpa_key_mgmt=WPA-PSK' \
    'rsn_pairwise=CCMP'
  printf 'wpa_passphrase=%s\n' "$setup_passphrase"
} > "$hostapd_tmp"
chown 0:"$networkgames_gid" "$hostapd_tmp"
chmod 0640 "$hostapd_tmp"
mv -f "$hostapd_tmp" "$hostapd_config"
if test -e "$setup_profile"; then
  unlink "$setup_profile"
fi

admin_token=$(tr -d '[:space:]' < "$root/etc/networkgames/admin.token")
test "${#admin_token}" -ge 12
setup_tmp=$(mktemp "$boot/.NETWORKGAMES-SETUP.XXXXXX")
{
  printf '%s\n' \
    'NetworkGames secure setup credentials' \
    '' \
    "Setup Wi-Fi SSID: $setup_ssid" \
    "Setup Wi-Fi password: $setup_passphrase" \
    'Management URL: https://10.77.0.1:9443/' \
    'Management username: admin' \
    "Management password: $admin_token" \
    '' \
    'Your browser will warn about the device-unique setup certificate.' \
    'Verify and accept it only while connected to the setup Wi-Fi.' \
    'To force recovery mode, create an empty file named' \
    'networkgames-recovery on this boot partition and reboot.'
} > "$setup_tmp"
mv -f "$setup_tmp" "$boot/NETWORKGAMES-SETUP.txt"

sync "$root" "$boot"
echo "NetworkGames card repair installed; credentials are in NETWORKGAMES-SETUP.txt"
