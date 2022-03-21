# SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

############# builder
FROM golang:1.17.8 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-networking-filter
COPY . .
RUN make install

############# gardener-extension-shoot-networking-filter
FROM alpine:3.15.0 AS gardener-extension-shoot-networking-filter

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-shoot-networking-filter /gardener-extension-shoot-networking-filter
ENTRYPOINT ["/gardener-extension-shoot-networking-filter"]
