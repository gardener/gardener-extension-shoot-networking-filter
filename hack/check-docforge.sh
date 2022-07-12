#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e

docCommitHash="12922abea4c5afa207cc9c09b18414cd2348890c"

echo "> Check Docforge Manifest"

repoPath="$(readlink -f $(dirname ${0})/..)"
manifestPath="${repoPath}/.docforge/manifest.yaml"
diffDirs=".docforge/;docs/"

tmpDir=$(mktemp -d)

function cleanup {
    rm -rf "$tmpDir"
}
trap cleanup EXIT

curl https://raw.githubusercontent.com/gardener/documentation/${docCommitHash}/.ci/check-manifest --output ${tmpDir}/check-manifest-script.sh && chmod +x ${tmpDir}/check-manifest-script.sh
curl https://raw.githubusercontent.com/gardener/documentation/${docCommitHash}/.ci/check-manifest-config --output ${tmpDir}/manifest-config
scriptPath="${tmpDir}/check-manifest-script.sh"
configPath="${tmpDir}/manifest-config"

${scriptPath} --repo-path ${repoPath} --repo-name "gardener-extension-shoot-networking-filter" --use-token false --manifest-path ${manifestPath} --diff-dirs ${diffDirs} --config-path ${configPath}
