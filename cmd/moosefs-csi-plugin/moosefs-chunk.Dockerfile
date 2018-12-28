# Copyright 2019 Tuxera Oy. All Rights Reserved.
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

# moosefs-chunk server in a single container
# TODO(anoop): Should be moved upstream
FROM ubuntu:18.04

# Install wget, lsb-release and curl
RUN apt-get update && apt-get install -y wget gnupg2

# Add key
RUN wget -O - http://ppa.moosefs.com/moosefs.key | apt-key add -
RUN . /etc/lsb-release && echo "deb http://ppa.moosefs.com/moosefs-3/apt/ubuntu/$DISTRIB_CODENAME $DISTRIB_CODENAME main" > /etc/apt/sources.list.d/moosefs.list

# Install chunk server
RUN apt-get update && apt-get install -y moosefs-chunkserver

# Config: used only while `docker build` the real mounts happens from k8s configMap
RUN mkdir -p /mnt/sdb1 && chown -R mfs:mfs /mnt/sdb1 && echo "/mnt/sdb1 1GiB" >> /etc/mfs/mfshdd.cfg && sed -i '/# LABELS =/c\LABELS = DOCKER' /etc/mfs/mfschunkserver.cfg

# Expose ports
EXPOSE 9422

# Start chunkserver in the foreground
CMD [ "mfschunkserver", "start", "-f" ]
