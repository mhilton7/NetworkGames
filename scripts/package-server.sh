#!/bin/bash
set -euo pipefail

version=0.1.0-rc.1
prefix="dist/wiibridge-host-${version}"
digest_ref=$(cat "${prefix}.digest")
manifest_digest=${digest_ref##*@}
GOCACHE="${GOCACHE:-/tmp/wiibridge-go-cache}" \
GOPATH="${GOPATH:-/tmp/wiibridge-gopath}" \
  go list -m -json all |
  jq -s --arg version "$version" '
    map({SPDXID:("SPDXRef-Package-"+(.Path|gsub("[^A-Za-z0-9.-]";"-"))),
      name:.Path,versionInfo:(.Version // "local"),
      downloadLocation:(.Origin.URL // "NOASSERTION"),filesAnalyzed:false,
      licenseConcluded:"NOASSERTION",licenseDeclared:"NOASSERTION"}) |
    {spdxVersion:"SPDX-2.3",dataLicense:"CC0-1.0",
     SPDXID:"SPDXRef-DOCUMENT",name:("wiibridge-host-"+$version),
     documentNamespace:("https://wiibridge.invalid/spdx/host/"+$version),
     creationInfo:{created:"2026-07-24T00:00:00Z",
       creators:["Tool: wiibridge-package-server"]},packages:.}' \
  > "${prefix}.sbom.spdx.json"
source_tree_sha=$(
  find server shared config scripts deploy docs -type f -print0 |
    sort -z | xargs -0 sha256sum | sha256sum | cut -d' ' -f1
)
if [ -z "$(git status --porcelain --untracked-files=normal)" ]; then
  source_worktree_dirty=false
else
  source_worktree_dirty=true
fi
jq -n --arg digest "$manifest_digest" --arg commit "$(git rev-parse HEAD)" \
  --arg source_tree_sha "$source_tree_sha" \
  --arg finished_on "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --argjson source_worktree_dirty "$source_worktree_dirty" \
  '{predicateType:"https://slsa.dev/provenance/v1",
    subject:[{name:"wiibridge-host",digest:{sha256:($digest|sub("^sha256:";""))}}],
    buildDefinition:{buildType:"wiibridge/reproducible-oci-layout",
      externalParameters:{os:"linux",architecture:"amd64"},
      resolvedDependencies:[{uri:"git+local:wiibridge",
        digest:{gitCommit:$commit,sourceTreeSha256:$source_tree_sha}}]},
    runDetails:{builder:{id:"scripts/build-oci.sh"},
      metadata:{finishedOn:$finished_on,
        sourceWorktreeDirty:$source_worktree_dirty}}}' \
  > "${prefix}.provenance.json"
sha256sum "${prefix}.oci/index.json" > "${prefix}.oci.index.sha256"
