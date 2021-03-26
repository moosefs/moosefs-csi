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

MFS3VER=3.0.115
MFS4VER=4.30.2
DRIVER_VERSION ?= 0.9.2
MFS3TAG=$(DRIVER_VERSION)-$(MFS3VER)
MFS4TAG=$(DRIVER_VERSION)-$(MFS4VER)
DEVTAG=$(DRIVER_VERSION)-dev

NAME=moosefs-csi-plugin
DOCKER_REGISTRY=registry.moosefs.pro:8443

ready: clean compile
publish: build push-image

compile:
	@echo "==> Building the project"
	@env CGO_ENABLED=0 GOCACHE=/tmp/go-cache GOOS=linux GOARCH=amd64 go build -a -o cmd/moosefs-csi-plugin/${NAME} cmd/moosefs-csi-plugin/main.go

build:
	@echo "==> Building the docker image"
	@docker build -t $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(DEVTAG) cmd/moosefs-csi-plugin
	@docker build -t $(DOCKER_REGISTRY)/moosefs-csi-plugin:latest cmd/moosefs-csi-plugin

push-image:
	@echo "==> Publishing $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(DEVTAG)"
	@docker push $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(DEVTAG)
	@docker push $(DOCKER_REGISTRY)/moosefs-csi-plugin:latest-dev
	@echo "==> Your image is now available at $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(DEVTAG)"

build-prod:
#	@echo "==> Building the docker image (PROD)"
#	@docker tag $(DOCKER_REGISTRY)/moosefs-csi-plugin:master $(DOCKER_REGISTRY)/moosefs-csi-plugin:master-backup
	@docker build -t $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS3TAG) cmd/moosefs-csi-plugin -f cmd/moosefs-csi-plugin/Dockerfile-mfs3-pro
	@docker build -t $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS4TAG) cmd/moosefs-csi-plugin -f cmd/moosefs-csi-plugin/Dockerfile-mfs4-pro

push-image-prod:
#	@echo "==> Publishing $(DOCKER_REGISTRY)/moosefs-csi-plugin:master"
	@docker push $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS3TAG)
	@docker push $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS4TAG)
#	@echo "==> Your image is now available at $(DOCKER_REGISTRY)/moosefs-csi-plugin:master"

clean:
	@echo "==> Cleaning releases"
	@GOOS=linux go clean -i -x ./...

.PHONY: clean