# Copyright 2021 Tappest sp. z o.o. All Rights Reserved.
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

MFS3VER=3.0.116
MFS4VER=4.32.0
DRIVER_VERSION ?= 0.9.2
MFS3TAGCE=$(DRIVER_VERSION)-$(MFS3VER)
MFS3TAGPRO=$(DRIVER_VERSION)-$(MFS3VER)-pro
MFS4TAGPRO=$(DRIVER_VERSION)-$(MFS4VER)-pro
DEVTAG=$(DRIVER_VERSION)-dev

NAME=moosefs-csi-plugin
DOCKER_REGISTRY=registry.moosefs.pro:8443

ready: clean compile
publish: build push-image

compile:
	@echo "==> Building the project"
	@env CGO_ENABLED=0 GOCACHE=/tmp/go-cache GOOS=linux GOARCH=amd64 go build -a -o cmd/moosefs-csi-plugin/${NAME} cmd/moosefs-csi-plugin/main.go

# todo: fix for dev version
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
	@docker build -t $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS3TAGCE) cmd/moosefs-csi-plugin -f cmd/moosefs-csi-plugin/Dockerfile-mfs3-ce
	@docker build -t $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS3TAGPRO) cmd/moosefs-csi-plugin -f cmd/moosefs-csi-plugin/Dockerfile-mfs3-pro
	@docker build -t $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS4TAGPRO) cmd/moosefs-csi-plugin -f cmd/moosefs-csi-plugin/Dockerfile-mfs4-pro

push-image-prod:
	@docker push $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS3TAGCE)
	@docker push $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS3TAGPRO)
	@docker push $(DOCKER_REGISTRY)/moosefs-csi-plugin:$(MFS4TAGPRO)

clean:
	@echo "==> Cleaning releases"
	@GOOS=linux go clean -i -x ./...

.PHONY: clean