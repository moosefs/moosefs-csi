# Copyright (c) 2024 Saglabs SA. All Rights Reserved.
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

MFS_VERSION = "4.56.6"
CSI_VERSION ?= "0.9.7"

MFS_TAG=$(CSI_VERSION)-$(MFS_VERSION)
DEV_TAG=$(CSI_VERSION)-dev

NAME=moosefs-csi

csi: clean compile
dev: build-dev push-dev
prod: build-prod push-prod

compile:
	@echo "==> Building the CSI driver"
	@env CGO_ENABLED=0 GOCACHE=/tmp/go-cache GOOS=linux GOARCH=amd64 go build -a -o cmd/moosefs-csi-plugin/${NAME} cmd/moosefs-csi-plugin/main.go

build-dev:
	@echo "==> Building DEV CSI images"
	@docker build --no-cache -t moosefs/$(NAME):dev -t moosefs/$(NAME):$(DEV_TAG) --build-arg MFS_TAG=v$(MFS_VERSION) --build-arg CSI_TAG=dev cmd/moosefs-csi-plugin

push-dev:
	@echo "==> Publishing DEV CSI image on hub.docker.com: moosefs/$(NAME):$(DEV_TAG)"
	@docker push moosefs/$(NAME):$(DEV_TAG)
	@docker push moosefs/$(NAME):dev

build-prod:
	@echo "==> Building Production CSI images"
	@docker build --no-cache -t moosefs/$(NAME):$(MFS_TAG) --build-arg MFS_TAG=v$(MFS_VERSION) --build-arg CSI_TAG=$(CSI_VERSION) cmd/moosefs-csi-plugin

push-prod:
	@echo "==> Publishing PRODUCTION CSI image on hub.docker.com: moosefs/$(NAME):$(MFS_TAG)"
	@docker push moosefs/$(NAME):$(MFS_TAGCE)

dev-buildx:
	@echo "==> Using buildx to build and publish dev image"
	@docker buildx build --no-cache --push --platform linux/amd64,linux/arm64,linux/arm/v7 --build-arg MFS_TAG=v$(MFS_VERSION) --build-arg CSI_TAG=dev -t moosefs/$(NAME):dev -t moosefs/$(NAME):$(DEV_TAG) cmd/moosefs-csi-plugin

prod-buildx:
	@echo "==> Using buildx to build and publish production image"
	@docker buildx build --push --platform linux/amd64,linux/arm64,linux/arm/v7 --build-arg MFS_TAG=v$(MFS_VERSION) --build-arg CSI_TAG=dev -t moosefs/$(NAME):$(MFS_TAG) cmd/moosefs-csi-plugin

clean:
	@echo "==> Cleaning releases"
	@GOOS=linux go clean -i -x ./...

.PHONY: clean
