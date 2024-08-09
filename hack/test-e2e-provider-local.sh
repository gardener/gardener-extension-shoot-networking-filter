#!/bin/bash

# SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o nounset
set -o pipefail
set -o errexit

repo_root="$(readlink -f $(dirname ${0})/..)"

if [[ ! -d "$repo_root/gardener" ]]; then
  git clone https://github.com/gardener/gardener.git
fi

gardener_version=$(go list -m -f '{{.Version}}' github.com/gardener/gardener)
cd "$repo_root/gardener"
git checkout "$gardener_version"
source "$repo_root/gardener/hack/ci-common.sh"
make kind-up
trap '{
  cd "$repo_root/gardener"
  export_artifacts "gardener-local"
  make kind-down
}' EXIT
export KUBECONFIG=$repo_root/gardener/example/gardener-local/kind/local/kubeconfig
make gardener-up

cd $repo_root

version=$(git rev-parse HEAD)
make docker-images
docker tag europe-docker.pkg.dev/gardener-project/public/gardener/extensions/shoot-networking-filter:latest shoot-networking-filter-local:$version
kind load docker-image shoot-networking-filter-local:$version --name gardener-local

mkdir -p $repo_root/tmp
cp -f $repo_root/example/controller-registration.yaml $repo_root/tmp/controller-registration.yaml
yq -i e "(select (.helm.values.image) | .helm.values.image.tag) |= \"$version\"" $repo_root/tmp/controller-registration.yaml
yq -i e '(select (.helm.values.image) | .helm.values.image.repository) |= "docker.io/library/shoot-networking-filter-local"' $repo_root/tmp/controller-registration.yaml

kubectl apply -f "$repo_root/tmp/controller-registration.yaml"

echo '172.18.255.1 api.e2e-default.local.external.local.gardener.cloud' >> /etc/hosts
echo '172.18.255.1 api.e2e-blackholing.local.external.local.gardener.cloud' >> /etc/hosts

# reduce flakiness in contended pipelines
export GOMEGA_DEFAULT_EVENTUALLY_TIMEOUT=5s
export GOMEGA_DEFAULT_EVENTUALLY_POLLING_INTERVAL=200ms
# if we're running low on resources, it might take longer for tested code to do something "wrong"
# poll for 5s to make sure, we're not missing any wrong action
export GOMEGA_DEFAULT_CONSISTENTLY_DURATION=5s
export GOMEGA_DEFAULT_CONSISTENTLY_POLLING_INTERVAL=200ms

ginkgo --timeout=1h --v --show-node-events "$@" $repo_root/test/e2e/...

cd "$repo_root/gardener"
make gardener-down
