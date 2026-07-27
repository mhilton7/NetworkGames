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
test "$(cat "$root/usr/share/wiibridge/board-target")" = zero-w-armhf
test -s "$root/etc/wiibridge/admin.token"
test -s "$root/etc/wiibridge/device.crt"
test -s "$root/etc/wiibridge/device.key"
test -x "$root/usr/sbin/hostapd"
test -x "$root/usr/sbin/dnsmasq"

project=$(cd "$(dirname "$0")/.." && pwd)
files=$project/pi/packaging/pi-gen/common/files
units=$project/pi/packaging/systemd
wiibridge_gid=$(
  awk -F: '$1=="wiibridge"{print $3; exit}' "$root/etc/group"
)
journal_gid=$(
  awk -F: '$1=="systemd-journal"{print $3; exit}' "$root/etc/group"
)
test -n "$wiibridge_gid"
test -n "$journal_gid"

install -o 0 -g 0 -m 0555 "$controller" \
  "$root/usr/bin/wiibridge-pi-controller"
install -o 0 -g 0 -m 0555 \
  "$files/wiibridge-helper" \
  "$files/wiibridge-firstboot" \
  "$files/wiibridge-gadget" \
  "$files/wiibridge-setup-ap" \
  "$files/wiibridge-provision" \
  "$root/usr/libexec/"
install -o 0 -g "$wiibridge_gid" -m 0440 \
  "$files/wiibridge-sudoers" \
  "$root/etc/sudoers.d/wiibridge"
install -o 0 -g 0 -m 0444 \
  "$units/wiibridge-firstboot.service" \
  "$units/wiibridge-controller.service" \
  "$units/wiibridge-auto-attach.service" \
  "$units/wiibridge-recover.service" \
  "$units/wiibridge-setup-ap.service" \
  "$units/wiibridge-setup-hostapd.service" \
  "$units/wiibridge-setup-dnsmasq.service" \
  "$root/etc/systemd/system/"
install -d -o 0 -g 0 -m 0755 \
  "$root/etc/systemd/journald.conf.d" \
  "$root/etc/modules-load.d" \
  "$root/etc/modprobe.d" \
  "$root/usr/lib/tmpfiles.d"
install -d -o 0 -g 0 -m 0700 \
  "$root/etc/NetworkManager/system-connections"
install -o 0 -g 0 -m 0644 "$files/wiibridge-journald.conf" \
  "$root/etc/systemd/journald.conf.d/wiibridge.conf"
install -o 0 -g 0 -m 0644 "$files/wiibridge-tmpfiles.conf" \
  "$root/usr/lib/tmpfiles.d/wiibridge.conf"
install -o 0 -g 0 -m 0644 "$files/wiibridge-modules-load.conf" \
  "$root/etc/modules-load.d/wiibridge.conf"
install -o 0 -g 0 -m 0644 "$files/wiibridge-modprobe.conf" \
  "$root/etc/modprobe.d/wiibridge-nbd.conf"
install -o 0 -g 0 -m 0644 \
  "$files/wiibridge-dnsmasq.conf" \
  "$root/etc/wiibridge-dnsmasq.conf"
if test -e "$root/etc/wiibridge/dnsmasq.conf"; then
  unlink "$root/etc/wiibridge/dnsmasq.conf"
fi
install -d -o 0 -g "$journal_gid" -m 2755 "$root/var/log/journal"

ln -sfn /etc/systemd/system/wiibridge-controller.service \
  "$root/etc/systemd/system/multi-user.target.wants/wiibridge-controller.service"
ln -sfn /etc/systemd/system/wiibridge-auto-attach.service \
  "$root/etc/systemd/system/multi-user.target.wants/wiibridge-auto-attach.service"
ln -sfn /etc/systemd/system/wiibridge-recover.service \
  "$root/etc/systemd/system/multi-user.target.wants/wiibridge-recover.service"
ln -sfn /etc/systemd/system/wiibridge-setup-ap.service \
  "$root/etc/systemd/system/multi-user.target.wants/wiibridge-setup-ap.service"
if test -L "$root/etc/systemd/system/multi-user.target.wants/hostapd.service"; then
  unlink "$root/etc/systemd/system/multi-user.target.wants/hostapd.service"
fi
ln -sfn /dev/null "$root/etc/systemd/system/hostapd.service"

chown 0:"$wiibridge_gid" "$root/etc/wiibridge"
chmod 0750 "$root/etc/wiibridge"
for credential in admin.token device.crt device.key; do
  chown 0:"$wiibridge_gid" "$root/etc/wiibridge/$credential"
  chmod 0640 "$root/etc/wiibridge/$credential"
done
if test -e "$root/etc/wiibridge/auto-attach" &&
  ! grep -Eq '^WIIBRIDGE_USB_VID=0x[0-9A-Fa-f]{4}$' \
    "$root/etc/wiibridge/bridge.env" ||
  test -e "$root/etc/wiibridge/auto-attach" &&
  ! grep -Eq '^WIIBRIDGE_USB_PID=0x[0-9A-Fa-f]{4}$' \
    "$root/etc/wiibridge/bridge.env"; then
  unlink "$root/etc/wiibridge/auto-attach"
fi
if test "$reset_admin" = --reset-admin-password; then
  admin_tmp=$(mktemp "$root/etc/wiibridge/.admin-token.XXXXXX")
  openssl rand -hex 6 > "$admin_tmp"
  chown 0:"$wiibridge_gid" "$admin_tmp"
  chmod 0640 "$admin_tmp"
  mv -f "$admin_tmp" "$root/etc/wiibridge/admin.token"
fi

printf '%s\n' "$country" > "$boot/wiibridge-country"
machine=$(tr -d '[:space:]' < "$root/etc/machine-id")
test "${#machine}" -ge 8
suffix=${machine:0:8}
setup_ssid=WiiBridge-"$suffix"
setup_profile=$root/etc/NetworkManager/system-connections/wiibridge-setup.nmconnection
hostapd_config=$root/etc/wiibridge/hostapd.conf
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
hostapd_tmp=$(mktemp "$root/etc/wiibridge/.hostapd.XXXXXX")
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
chown 0:"$wiibridge_gid" "$hostapd_tmp"
chmod 0640 "$hostapd_tmp"
mv -f "$hostapd_tmp" "$hostapd_config"
if test -e "$setup_profile"; then
  unlink "$setup_profile"
fi

admin_token=$(tr -d '[:space:]' < "$root/etc/wiibridge/admin.token")
test "${#admin_token}" -ge 12
setup_tmp=$(mktemp "$boot/.WIIBRIDGE-SETUP.XXXXXX")
{
  printf '%s\n' \
    'WiiBridge secure setup credentials' \
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
    'wiibridge-recovery on this boot partition and reboot.'
} > "$setup_tmp"
mv -f "$setup_tmp" "$boot/WIIBRIDGE-SETUP.txt"

sync "$root" "$boot"
echo "WiiBridge card repair installed; credentials are in WIIBRIDGE-SETUP.txt"
