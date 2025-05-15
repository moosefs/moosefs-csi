# Copyright (c) 2025 Saglabs SA. All Rights Reserved.
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

ARG CSI_TAG="v0.9.8"
ARG MFS_TAG="v4.57.6"

#Build MooseFS CSI driver from source
FROM golang:1.23-bookworm AS csibuilder
WORKDIR /build
ARG CSI_TAG
RUN git clone --depth 1 --branch ${CSI_TAG} https://github.com/moosefs/moosefs-csi.git
RUN cd moosefs-csi && CGO_ENABLED=0 GOCACHE=/tmp/go-cache GOOS=linux go build -a -o /build/moosefs-csi-plugin cmd/moosefs-csi-plugin/main.go

#Build MooseFS Client from source Debian 12 Bookworm
# MooseFS client is required for the CSI driver to mount volumes
FROM ghcr.io/moosefs/mfsbuilder:latest AS mfsbuilder
WORKDIR /moosefs
ARG MFS_TAG
RUN git clone --depth 1 --branch ${MFS_TAG} https://github.com/moosefs/moosefs.git /moosefs
RUN autoreconf -f -i
RUN ./configure --prefix=/usr --mandir=/share/man --sysconfdir=/etc --localstatedir=/var/lib --with-default-user=mfs --with-default-group=mfs --disable-mfsbdev --disable-mfsmaster --disable-mfschunkserver --disable-mfsmetalogger --disable-mfsnetdump --disable-mfscgi --disable-mfscgiserv --disable-mfscli
RUN cd /moosefs/mfsclient && make DESTDIR=/tmp/ install

#Build CSI plugin container
FROM debian:bookworm
RUN apt update && apt install -y libfuse3-3
COPY --from=csibuilder /build/moosefs-csi-plugin /bin/moosefs-csi-plugin
COPY --from=mfsbuilder /tmp/usr/bin /usr/bin
RUN ["ln", "-s", "/usr/bin/mfsmount", "/usr/sbin/mount.moosefs"]
ENTRYPOINT ["/bin/moosefs-csi-plugin"]
