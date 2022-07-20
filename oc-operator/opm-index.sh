#!/usr/bin/env bash
set -euo pipefail

VERSION=$(yq e '.images[0].newTag | sub("v","")' ../config/manager/kustomization.yaml)

list=""
for i in $(ls -d tf-controller/${VERSION}/ | xargs -I{} basename {}); do
  # docker build and push individual bundles
  docker build -t quay.io/openshift-fluxv2-poc/tf-controller-catalog:bundle-v"${i}" -f bundle.Dockerfile tf-controller/"${i}"
  docker push quay.io/openshift-fluxv2-poc/tf-controller-catalog:bundle-v"${i}"
  list="$list,quay.io/openshift-fluxv2-poc/tf-controller-catalog:bundle-v$i"
done

docker build -t opm -f Dockerfile.opm .

list=${list:1} # remove first comma
docker run --rm -it \
  --privileged \
  -v /var/lib/docker:/var/lib/docker \
  -v /var/run/docker.sock:/var/run/docker.sock \
  opm:latest index add \
  --container-tool docker \
  --bundles "$list" \
  --tag quay.io/openshift-fluxv2-poc/tf-controller-index:latest

# push index
docker push quay.io/openshift-fluxv2-poc/tf-controller-index:latest
