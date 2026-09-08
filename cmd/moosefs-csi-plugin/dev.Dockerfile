# Copyright (c) 2026 Saglabs SA. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ARG GO_VERSION="1.27.1"

FROM golang:${GO_VERSION}-trixie AS csibuilder
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/moosefs-csi-plugin/main.go ./cmd/moosefs-csi-plugin/main.go
COPY driver ./driver
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/moosefs-csi-plugin ./cmd/moosefs-csi-plugin

FROM debian:13-slim
ARG MFS_VERSION="4.59.2-1"
ARG MFS_APT_REPOSITORY="https://repository.moosefs.com/moosefs-4/apt/debian/trixie"
ARG DEBIAN_FRONTEND="noninteractive"

RUN set -eux; \
    printf 'path-exclude=/usr/share/doc/*\npath-exclude=/usr/share/man/*\npath-exclude=/usr/share/info/*\n' \
        > /etc/dpkg/dpkg.cfg.d/01_nodoc; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl; \
    curl -fsSL https://repository.moosefs.com/moosefs.key \
        -o /usr/share/keyrings/moosefs.asc; \
    printf 'deb [signed-by=/usr/share/keyrings/moosefs.asc] %s trixie main\n' "${MFS_APT_REPOSITORY}" \
        > /etc/apt/sources.list.d/moosefs.list; \
    apt-get update; \
    apt-get install -y --no-install-recommends fuse3 "moosefs-client=${MFS_VERSION}"; \
    ln -sf /usr/bin/mfsmount /usr/sbin/mount.moosefs; \
    apt-get purge -y --auto-remove ca-certificates curl; \
    rm -rf /etc/mfs /usr/share/doc/*/examples /usr/share/keyrings/moosefs.asc \
        /etc/apt/sources.list.d/moosefs.list /var/lib/apt/lists/* /var/cache/apt/*

COPY --from=csibuilder /out/moosefs-csi-plugin /bin/moosefs-csi-plugin
ENTRYPOINT ["/bin/moosefs-csi-plugin"]
