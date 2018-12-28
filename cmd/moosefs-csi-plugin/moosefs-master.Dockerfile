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
# moosefs-master in a single container

# TODO(anoop): Should be moved upstream
FROM ubuntu:18.04

# As much lesser layers as possible.
RUN apt-get update && \
    apt-get install -y wget && \
    wget http://ppa.moosefs.com/moosefs-3/apt/ubuntu/bionic/pool/main/m/moosefs/moosefs-master_3.0.101-1_amd64.deb && \
    dpkg -i moosefs-master_3.0.101-1_amd64.deb

# Expose master ports
EXPOSE 9419 9420 9421

# Start master server in the foreground
CMD [ "mfsmaster", "start", "-f" ]
