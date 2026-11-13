# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

############# builder
FROM golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d AS builder

WORKDIR /go/src/github.com/gardener/gardener-extension-shoot-networking-filter

# Copy go mod and sum files
COPY go.mod go.sum ./
# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

COPY . .

ARG EFFECTIVE_VERSION
RUN make install EFFECTIVE_VERSION=$EFFECTIVE_VERSION

############# gardener-extension-shoot-networking-filter
FROM  gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931 AS gardener-extension-shoot-networking-filter
WORKDIR /

COPY charts /charts
COPY --from=builder /go/bin/gardener-extension-shoot-networking-filter /gardener-extension-shoot-networking-filter
ENTRYPOINT ["/gardener-extension-shoot-networking-filter"]

FROM  gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931 AS gardener-extension-shoot-networking-filter-admission
WORKDIR /

COPY --from=builder /go/bin/gardener-extension-shoot-networking-filter-admission /gardener-extension-shoot-networking-filter-admission
ENTRYPOINT ["/gardener-extension-shoot-networking-filter-admission"]