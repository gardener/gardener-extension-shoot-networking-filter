# SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

############# builder
FROM golang:1.25.4 AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-networking-filter

# Copy go mod and sum files
COPY go.mod go.sum ./
# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

COPY . .

ARG EFFECTIVE_VERSION
RUN make install EFFECTIVE_VERSION=$EFFECTIVE_VERSION

############# gardener-extension-shoot-networking-filter
FROM  gcr.io/distroless/static-debian12:nonroot AS gardener-extension-shoot-networking-filter
WORKDIR /

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-shoot-networking-filter /gardener-extension-shoot-networking-filter
ENTRYPOINT ["/gardener-extension-shoot-networking-filter"]

############ gardener-runtime-networking-filter
FROM  gcr.io/distroless/static-debian12:nonroot AS gardener-runtime-networking-filter
WORKDIR /

COPY --from=builder /go/bin/gardener-runtime-networking-filter /gardener-runtime-networking-filter
ENTRYPOINT ["/gardener-runtime-networking-filter"]

############ gardener-runtime-networking-filter
FROM  gcr.io/distroless/static-debian12:nonroot AS gardener-extension-shoot-networking-filter-admission
WORKDIR /

COPY --from=builder /go/bin/gardener-extension-shoot-networking-filter-admission /gardener-extension-shoot-networking-filter-admission
ENTRYPOINT ["/gardener-extension-shoot-networking-filter-admission"]