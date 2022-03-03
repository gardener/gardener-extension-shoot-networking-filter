# SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

############# builder
FROM golang:1.17.5 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-networking-policy-filter
COPY . .
RUN make install

############# gardener-extension-networking-policy-filter
FROM alpine:3.15.0 AS gardener-extension-networking-policy-filter

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-networking-policy-filter /gardener-extension-networking-policy-filter
ENTRYPOINT ["/gardener-extension-networking-policy-filter"]
