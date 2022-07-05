# SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

############# builder
FROM golang:1.18.3 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-networking-filter
COPY . .
RUN make install

############# gardener-extension-shoot-networking-filter
FROM  gcr.io/distroless/static-debian11:nonroot AS gardener-extension-shoot-networking-filter
WORKDIR /

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-shoot-networking-filter /gardener-extension-shoot-networking-filter
ENTRYPOINT ["/gardener-extension-shoot-networking-filter"]
