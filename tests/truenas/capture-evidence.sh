#!/bin/sh
set -eu

container=${1:?container name required}
out=${2:-reports/truenas}
mkdir -p "$out"
docker inspect "$container" |
  jq '.[0] | {Config:{User:.Config.User,Healthcheck:.Config.Healthcheck},
  HostConfig:{ReadonlyRootfs:.HostConfig.ReadonlyRootfs,
  Privileged:.HostConfig.Privileged,CapDrop:.HostConfig.CapDrop,
  SecurityOpt:.HostConfig.SecurityOpt,Devices:.HostConfig.Devices,
  Binds:.HostConfig.Binds,PortBindings:.HostConfig.PortBindings},
  State:.State,Mounts:.Mounts}' > "$out/container-inspect.json"
docker logs --since 10m "$container" 2>&1 |
  sed -E 's/(token|password|private_key|psk)=[^ ]+/\1=REDACTED/Ig' \
  > "$out/container.log"
docker exec "$container" /wiibridge-host healthcheck > "$out/health.txt"
