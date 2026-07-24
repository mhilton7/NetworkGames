#!/bin/bash
set -euo pipefail

target=${1:?target}
version=0.1.0-rc.1
prefix="dist/networkgames-hostbridge-${version}-${target}"
image="${prefix}.img"
test -s "$image"
"tests/firmware/offline/validate.sh" "$target"
sha256sum "$image" > "${image}.sha256"
bmaptool create -o "${prefix}.bmap" "$image"
xz -T0 -9 --keep --force "$image"
sha256sum "${image}.xz" > "${image}.xz.sha256"
original=$(cut -d' ' -f1 < "${image}.sha256")
expanded=$(xz -dc "${image}.xz" | sha256sum | cut -d' ' -f1)
test "$original" = "$expanded"
packages="${prefix}.packages.txt"
sbom="${prefix}.sbom.spdx.json"
jq -Rn --arg target "$target" --arg version "$version" \
  '[inputs | select(length>0) | split("=") | {
    SPDXID:("SPDXRef-Package-"+(.[0]|gsub("[^A-Za-z0-9.-]";"-"))),
    name:.[0],versionInfo:(.[1:]|join("=")),downloadLocation:"NOASSERTION",
    filesAnalyzed:false,licenseConcluded:"NOASSERTION",licenseDeclared:"NOASSERTION"}] |
  {spdxVersion:"SPDX-2.3",dataLicense:"CC0-1.0",
   SPDXID:"SPDXRef-DOCUMENT",name:("networkgames-hostbridge-"+$target),
   documentNamespace:("https://networkgames.invalid/spdx/"+$version+"/"+$target),
   creationInfo:{created:"2026-07-24T00:00:00Z",
     creators:["Tool: networkgames-package-firmware"]},packages:.}' \
  < "$packages" > "$sbom"
image_sha=$(cut -d' ' -f1 < "${image}.sha256")
xz_sha=$(cut -d' ' -f1 < "${image}.xz.sha256")
source_tree_sha=$(
  find server pi shared config scripts tests deploy docs -type f -print0 |
    sort -z | xargs -0 sha256sum | sha256sum | cut -d' ' -f1
)
jq -n --arg target "$target" --arg image_sha "$image_sha" --arg xz_sha "$xz_sha" \
  --arg app_commit "$(git rev-parse HEAD)" \
  --arg source_tree_sha "$source_tree_sha" \
  --arg finished_on "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg pi_gen_armhf "314262cb286b8f33327a6f0cbabe14c625021ca0" \
  --arg pi_gen_arm64 "ca8aeed0ae300c2a89f55ce9617d5f96a27e99e5" \
  '{predicateType:"https://slsa.dev/provenance/v1",
    subject:[{name:($target+".img"),digest:{sha256:$image_sha}},
             {name:($target+".img.xz"),digest:{sha256:$xz_sha}}],
    buildDefinition:{buildType:"networkgames/pi-gen",
      externalParameters:{target:$target},
      resolvedDependencies:[
       {uri:"git+https://github.com/RPi-Distro/pi-gen@master",digest:{gitCommit:$pi_gen_armhf}},
       {uri:"git+https://github.com/RPi-Distro/pi-gen@arm64",digest:{gitCommit:$pi_gen_arm64}},
       {uri:"git+local:networkgames",
        digest:{gitCommit:$app_commit,sourceTreeSha256:$source_tree_sha}}]},
    runDetails:{builder:{id:"networkgames-build-firmware.sh"},
      metadata:{invocationId:($target+"-20260724"),finishedOn:$finished_on,
        sourceWorktreeDirty:true}}}' \
  > "${prefix}.provenance.json"
report_dir="reports/firmware/${target}"
mkdir -p "$report_dir"
cp "${prefix}.offline-validation.json" "${prefix}.packages.txt" \
  "${prefix}.sbom.spdx.json" "${prefix}.provenance.json" \
  "${prefix}.build.log" "$report_dir/"
