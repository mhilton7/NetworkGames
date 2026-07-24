#!/bin/bash
set -euo pipefail

root=$(mktemp -d)
port=21089
host_pid=
mkdir -p reports
cleanup() {
  test ! -f "$root/host.log" || cp "$root/host.log" reports/libnbd-host.log
  test ! -f "$root/nbd-error" || cp "$root/nbd-error" reports/libnbd-error.log
  if test -n "$host_pid"; then kill "$host_pid" 2>/dev/null || true; wait "$host_pid" 2>/dev/null || true; fi
  chmod -R u+rwX "$root" 2>/dev/null || true
  rm -rf "$root"
}
trap cleanup EXIT
mkdir -p "$root"/{library,data,certs,client}
go run ./scripts/synthetic-wbfs "$root/library/TEST03.wbfs" TEST03
printf 'cn=interop-ca\nca\ncert_signing_key\nexpiration_days=2\n' > "$root/ca.info"
certtool --generate-privkey --outfile "$root/ca.key" >/dev/null 2>&1
certtool --generate-self-signed --load-privkey "$root/ca.key" \
  --template "$root/ca.info" --outfile "$root/certs/ca.crt" >/dev/null 2>&1
printf 'cn=localhost\ndns_name=localhost\nip_address=127.0.0.1\ntls_www_server\nencryption_key\nsigning_key\nexpiration_days=2\n' > "$root/server.info"
certtool --generate-privkey --outfile "$root/certs/server.key" >/dev/null 2>&1
certtool --generate-certificate --load-ca-certificate "$root/certs/ca.crt" \
  --load-ca-privkey "$root/ca.key" --load-privkey "$root/certs/server.key" \
  --template "$root/server.info" --outfile "$root/certs/server.crt" >/dev/null 2>&1
printf 'cn=pi-interop\ntls_www_client\nencryption_key\nsigning_key\nexpiration_days=2\n' > "$root/client.info"
certtool --generate-privkey --outfile "$root/client/client-key.pem" >/dev/null 2>&1
certtool --generate-certificate --load-ca-certificate "$root/certs/ca.crt" \
  --load-ca-privkey "$root/ca.key" --load-privkey "$root/client/client-key.pem" \
  --template "$root/client.info" --outfile "$root/client/client-cert.pem" >/dev/null 2>&1
cp "$root/certs/ca.crt" "$root/certs/clients-ca.crt"
cp "$root/certs/ca.crt" "$root/client/ca-cert.pem"
chmod 0555 "$root/library"
chmod 0444 "$root/library/TEST03.wbfs"
NETWORKGAMES_LIBRARY="$root/library" NETWORKGAMES_DATA="$root/data" \
NETWORKGAMES_ADMIN_TOKEN=0123456789abcdef0123456789abcdef \
NETWORKGAMES_TLS_CERT="$root/certs/server.crt" \
NETWORKGAMES_TLS_KEY="$root/certs/server.key" \
NETWORKGAMES_TLS_CLIENT_CA="$root/certs/clients-ca.crt" \
NETWORKGAMES_NBD_LISTEN="127.0.0.1:$port" \
NETWORKGAMES_HTTPS_LISTEN=127.0.0.1:21443 \
  build/bin/networkgames-host > "$root/host.log" 2>&1 &
host_pid=$!
for _ in $(seq 1 50); do
  if LIBNBD_DEBUG=1 \
    nbdinfo "nbds://pi-interop@localhost:$port/all?tls-certificates=$root/client" \
    > reports/libnbd-info.json 2> "$root/nbd-error"; then break; fi
  sleep 0.1
done
nbdinfo --is read-only "nbds://pi-interop@localhost:$port/all?tls-certificates=$root/client"
if nbdinfo "nbd://localhost:$port/all" >/dev/null 2>&1; then
  echo "plaintext NBD unexpectedly succeeded" >&2
  exit 1
fi
if empty=$(mktemp); then
  if nbdcopy "$empty" "nbds://pi-interop@localhost:$port/all?tls-certificates=$root/client" \
    >/dev/null 2>&1; then
    echo "write unexpectedly succeeded" >&2
    rm -f "$empty"
    exit 1
  fi
  rm -f "$empty"
fi
kill "$host_pid"
wait "$host_pid"
host_pid=
