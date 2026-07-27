#!/bin/bash
set -euo pipefail

version=0.1.0-rc.1
root="build/container-test-$$"
image=wiibridge-host:${version}
container=wiibridge-host-test
mkdir -p reports
mkdir -p "$root"/{library,config,data,certs,logs,backups}
GOCACHE="${GOCACHE:-/tmp/wiibridge-go-cache}" \
GOPATH="${GOPATH:-/tmp/wiibridge-gopath}" make server
chmod u+w build/container-root/wiibridge-host
cp build/bin/wiibridge-host build/container-root/wiibridge-host
sudo docker build --network=none -t "$image" -f server/packaging/container/Dockerfile .
go run ./scripts/synthetic-wbfs "$root/library/TEST01.wbfs" TEST01
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj /CN=wiibridge-test-ca \
  -keyout "$root/certs/ca.key" -out "$root/certs/ca.crt" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj /CN=localhost \
  -keyout "$root/certs/server.key" -out "$root/certs/server.csr" >/dev/null 2>&1
printf 'subjectAltName=DNS:localhost,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n' > "$root/certs/server.ext"
openssl x509 -req -days 2 -in "$root/certs/server.csr" \
  -CA "$root/certs/ca.crt" -CAkey "$root/certs/ca.key" -CAcreateserial \
  -extfile "$root/certs/server.ext" -out "$root/certs/server.crt" >/dev/null 2>&1
cp "$root/certs/ca.crt" "$root/certs/clients-ca.crt"
rm "$root/certs/ca.key" "$root/certs/server.csr" "$root/certs/server.ext" "$root/certs/ca.srl"
chmod -R a+rX "$root"
chmod a+rwx "$root"/{config,data,logs,backups}
before=$(sha256sum "$root/library/TEST01.wbfs")
sudo docker rm -f "$container" >/dev/null 2>&1 || true
sudo docker run -d --name "$container" \
  --user 568:568 --read-only --cap-drop ALL \
  --security-opt no-new-privileges:true --pids-limit 256 \
  --tmpfs /tmp:rw,noexec,nosuid,nodev,size=32m \
  -e WIIBRIDGE_ADMIN_TOKEN=0123456789abcdef0123456789abcdef \
  -v "$PWD/$root/library:/library:ro" \
  -v "$PWD/$root/config:/config:rw" \
  -v "$PWD/$root/data:/data:rw" \
  -v "$PWD/$root/certs:/certs:ro" \
  -v "$PWD/$root/logs:/logs:rw" \
  -v "$PWD/$root/backups:/backups:rw" "$image" >/dev/null
for _ in $(seq 1 30); do
  state=$(sudo docker inspect -f '{{.State.Status}}' "$container")
  test "$state" = running && break
  sleep 1
done
sudo docker exec "$container" /wiibridge-host healthcheck
if sudo docker exec "$container" /wiibridge-host scan --library /library >/dev/null; then :; else exit 1; fi
if sudo docker exec "$container" /wiibridge-host version >/dev/null; then :; fi
after=$(sha256sum "$root/library/TEST01.wbfs")
test "$before" = "$after"
sudo docker inspect "$container" |
  jq '.[0] | {image:.Image,user:.Config.User,health:.State.Health,
    readonly_root:.HostConfig.ReadonlyRootfs,privileged:.HostConfig.Privileged,
    cap_drop:.HostConfig.CapDrop,security_opt:.HostConfig.SecurityOpt,
    devices:.HostConfig.Devices,mounts:.Mounts}' > reports/container-test.json
sudo docker restart "$container" >/dev/null
for _ in $(seq 1 30); do
  test "$(sudo docker inspect -f '{{.State.Status}}' "$container")" = running && break
  sleep 1
done
test -n "$(sudo find "$root/data/snapshots" -type f -name '*.json' -print -quit)"
after_restart=$(sha256sum "$root/library/TEST01.wbfs")
test "$before" = "$after_restart"
sudo docker rm -f "$container" >/dev/null
test "$(sha256sum "$root/library/TEST01.wbfs")" = "$before"
image_id=$(sudo docker image inspect "$image" --format '{{.Id}}')
printf '%s\n' "$image_id" > reports/container-image-digest.txt
