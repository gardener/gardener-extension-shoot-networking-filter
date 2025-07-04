#!/bin/bash
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -eu

repo_root="$(readlink -f $(dirname ${0})/..)"
version=$(cat "${repo_root}/VERSION")

cat << EOF > "${repo_root}/example/shoot-networking-filter/example/extension-patch.yaml"
# DO NOT EDIT THIS FILE!
# This file is auto-generated by hack/prepare-operator-extension.sh.

apiVersion: operator.gardener.cloud/v1alpha1
kind: Extension
metadata:
  name: extension-shoot-networking-filter
spec:
  deployment:
    extension:
      helm:
        ociRepository:
          ref: europe-docker.pkg.dev/gardener-project/releases/charts/gardener/extensions/shoot-networking-filter:$version
EOF

kubectl kustomize "${repo_root}/example/shoot-networking-filter/example" -o "${repo_root}/example/extension-shoot-networking-filter.yaml"
