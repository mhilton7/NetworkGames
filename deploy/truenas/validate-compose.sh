#!/bin/sh
set -eu

compose=${1:-deploy/truenas/compose.yaml}
test -r "$compose"
python3 - "$compose" <<'PY'
import re, sys
text=open(sys.argv[1], encoding="utf-8").read()
required=("services:", "read_only: true", "cap_drop:", "no-new-privileges:true",
          "healthcheck:", "target: /library", "target: /certs")
missing=[x for x in required if x not in text]
if missing:
    raise SystemExit("missing required Compose properties: "+", ".join(missing))
if re.search(r'privileged\s*:\s*true|/var/run/docker\.sock|/dev/', text):
    raise SystemExit("prohibited privilege, Docker socket, or device access")
library=text[text.index("target: /library")-180:text.index("target: /library")+100]
if "read_only: true" not in library:
    raise SystemExit("/library is not read-only")
print("static compose policy: PASS")
PY
if command -v docker >/dev/null 2>&1; then
  docker compose --env-file deploy/truenas/.env.example -f "$compose" config >/dev/null
  echo "docker compose parser: PASS"
elif command -v podman-compose >/dev/null 2>&1; then
  podman-compose -f "$compose" config >/dev/null
  echo "podman-compose parser: PASS"
else
  echo "independent Compose parser unavailable" >&2
  exit 3
fi
