#!/bin/sh
set -eu

version=${PROJECT_VERSION:-0.1.0-rc.1}
out=${1:-dist/networkgames-host-${version}.oci}
root=build/container-root
cache=${GOCACHE:-/tmp/networkgames-go-cache}
gopath=${GOPATH:-/tmp/networkgames-gopath}
mkdir -p "$root" "$out/blobs/sha256"
GOCACHE="$cache" GOPATH="$gopath" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w -buildid=" \
  -o "$root/networkgames-host" ./server/host-daemon
chmod 0555 "$root/networkgames-host"
layer_tmp=$(mktemp)
config_tmp=$(mktemp)
manifest_tmp=$(mktemp)
trap 'rm -f "$layer_tmp" "$config_tmp" "$manifest_tmp"' EXIT
tar --sort=name --mtime='UTC 2026-07-24' --owner=568 --group=568 \
  --numeric-owner -C "$root" -cf "$layer_tmp" networkgames-host
layer_digest=$(sha256sum "$layer_tmp" | cut -d' ' -f1)
layer_size=$(wc -c < "$layer_tmp")
mv "$layer_tmp" "$out/blobs/sha256/$layer_digest"
printf '{"architecture":"amd64","os":"linux","config":{"User":"568:568","Entrypoint":["/networkgames-host"],"ExposedPorts":{"8443/tcp":{},"10809/tcp":{}},"Env":["PATH=/"]},"rootfs":{"type":"layers","diff_ids":["sha256:%s"]},"history":[{"created":"2026-07-24T00:00:00Z","created_by":"networkgames reproducible OCI builder"}]}' "$layer_digest" > "$config_tmp"
config_digest=$(sha256sum "$config_tmp" | cut -d' ' -f1)
config_size=$(wc -c < "$config_tmp")
mv "$config_tmp" "$out/blobs/sha256/$config_digest"
printf '{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:%s","size":%s},"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar","digest":"sha256:%s","size":%s}],"annotations":{"org.opencontainers.image.title":"networkgames-host","org.opencontainers.image.version":"%s","org.opencontainers.image.revision":"%s"}}' "$config_digest" "$config_size" "$layer_digest" "$layer_size" "$version" "$(git rev-parse HEAD)" > "$manifest_tmp"
manifest_digest=$(sha256sum "$manifest_tmp" | cut -d' ' -f1)
manifest_size=$(wc -c < "$manifest_tmp")
mv "$manifest_tmp" "$out/blobs/sha256/$manifest_digest"
printf '{"imageLayoutVersion":"1.0.0"}\n' > "$out/oci-layout"
printf '{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:%s","size":%s,"annotations":{"org.opencontainers.image.ref.name":"networkgames-host:%s"}}]}\n' "$manifest_digest" "$manifest_size" "$version" > "$out/index.json"
printf 'networkgames-host:%s@sha256:%s\n' "$version" "$manifest_digest" | tee "dist/networkgames-host-${version}.digest"
