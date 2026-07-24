#!/bin/bash
set -euo pipefail

root=$(mktemp -d)
port=21090
host_pid=
device=/dev/nbd0
cleanup() {
  sudo umount "$root/mnt" 2>/dev/null || true
  sudo /usr/sbin/nbd-client -d "$device" 2>/dev/null || true
  test -z "$host_pid" || kill "$host_pid" 2>/dev/null || true
  test -z "$host_pid" || wait "$host_pid" 2>/dev/null || true
  chmod -R u+rwX "$root" 2>/dev/null || true
  rm -rf "$root"
}
trap cleanup EXIT
mkdir -p "$root"/{library,data,certs,client,mnt}
go run ./scripts/synthetic-wbfs "$root/library/TEST04.wbfs" TEST04
printf 'cn=interop-ca\nca\ncert_signing_key\nexpiration_days=2\n' > "$root/ca.info"
certtool --generate-privkey --outfile "$root/ca.key" >/dev/null 2>&1
certtool --generate-self-signed --load-privkey "$root/ca.key" \
  --template "$root/ca.info" --outfile "$root/certs/ca.crt" >/dev/null 2>&1
printf 'cn=localhost\ndns_name=localhost\nip_address=127.0.0.1\ntls_www_server\nencryption_key\nsigning_key\nexpiration_days=2\n' > "$root/server.info"
certtool --generate-privkey --outfile "$root/certs/server.key" >/dev/null 2>&1
certtool --generate-certificate --load-ca-certificate "$root/certs/ca.crt" \
  --load-ca-privkey "$root/ca.key" --load-privkey "$root/certs/server.key" \
  --template "$root/server.info" --outfile "$root/certs/server.crt" >/dev/null 2>&1
printf 'cn=pi-kernel\ntls_www_client\nencryption_key\nsigning_key\nexpiration_days=2\n' > "$root/client.info"
certtool --generate-privkey --outfile "$root/client/client-key.pem" >/dev/null 2>&1
certtool --generate-certificate --load-ca-certificate "$root/certs/ca.crt" \
  --load-ca-privkey "$root/ca.key" --load-privkey "$root/client/client-key.pem" \
  --template "$root/client.info" --outfile "$root/client/client-cert.pem" >/dev/null 2>&1
cp "$root/certs/ca.crt" "$root/certs/clients-ca.crt"
chmod 0555 "$root/library"
chmod 0444 "$root/library/TEST04.wbfs"
NETWORKGAMES_LIBRARY="$root/library" NETWORKGAMES_DATA="$root/data" \
NETWORKGAMES_ADMIN_TOKEN=0123456789abcdef0123456789abcdef \
NETWORKGAMES_TLS_CERT="$root/certs/server.crt" \
NETWORKGAMES_TLS_KEY="$root/certs/server.key" \
NETWORKGAMES_TLS_CLIENT_CA="$root/certs/clients-ca.crt" \
NETWORKGAMES_NBD_LISTEN="127.0.0.1:$port" \
NETWORKGAMES_HTTPS_LISTEN=127.0.0.1:21444 \
  build/bin/networkgames-host > "$root/host.log" 2>&1 &
host_pid=$!
for _ in $(seq 1 50); do
  if nbdinfo "nbds://localhost:$port/all?tls-certificates=$root/client" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
sudo modprobe nbd max_part=8
sudo /usr/sbin/nbd-client -N all -R -x \
  -F "$root/client/client-cert.pem" -K "$root/client/client-key.pem" \
  -A "$root/certs/ca.crt" -H localhost \
  127.0.0.1 "$port" "$device"
test "$(sudo blockdev --getro "$device")" = 1
sudo mount -o ro "${device}p1" "$root/mnt"
test -f "$root/mnt/wbfs/TEST04.wbfs"
if sudo dd if=/dev/zero of="$device" bs=512 count=1 conv=notrunc status=none 2>/dev/null; then
  echo "kernel client unexpectedly accepted a write" >&2
  exit 1
fi
sudo umount "$root/mnt"
sudo /usr/sbin/nbd-client -d "$device"
kill "$host_pid"
wait "$host_pid"
host_pid=
