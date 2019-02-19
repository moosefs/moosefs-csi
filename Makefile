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

VERSION ?= dev
NAME=moosefs-csi-plugin

all: ready

ready: clean cred test compile

publish: ready build push-image

cred:
	@echo "==> Scanning secrets in commit history (prevent accidents)"
	# trufflehog --regex --entropy=False --rules scripts/truffleHogRegexes.json  file:///$(shell pwd)
	trufflehog --regex --entropy=False file:///$(shell pwd)

compile:
	@echo "==> Building the project"
	@env CGO_ENABLED=0 GOCACHE=/tmp/go-cache GOOS=linux GOARCH=amd64 go build -a -o cmd/moosefs-csi-plugin/${NAME} cmd/moosefs-csi-plugin/main.go

test:
	@echo "==> Running tests"
	go test -v ./driver/...

build:
	@echo "==> Building the docker image"
	@docker build -t quay.io/tuxera/moosefs-csi-plugin:$(VERSION) cmd/moosefs-csi-plugin
	@docker build -t quay.io/tuxera/moosefs-csi-plugin:latest cmd/moosefs-csi-plugin

push-image:
	@echo "==> Publishing tuxera/moosefs-csi-plugin:$(VERSION)"
	@docker push quay.io/tuxera/moosefs-csi-plugin:$(VERSION)
	@docker push quay.io/tuxera/moosefs-csi-plugin:latest
	@echo "==> Your image is now available at quay.io/tuxera/moosefs-csi-plugin:$(VERSION)/latest"

clean:
	@echo "==> Cleaning releases"
	@GOOS=linux go clean -i -x ./...

.PHONY: all push fetch build-image clean

# Builds moosefs-master, moosefs-chunk
# TODO(anoop): To be moved upstream
push-mfs-master:
	@echo "==> Building the quay.io/tuxera/moosefs-master docker image"
	@docker build -t quay.io/tuxera/moosefs-master:$(VERSION) -f moosefs-master.Dockerfile .
	@docker build -t quay.io/tuxera/moosefs-master:latest -f moosefs-master.Dockerfile .
	@echo "==> Publishing quay.io/tuxera/moosefs-master:$(VERSION)"
	@docker push quay.io/tuxera/moosefs-master:$(VERSION)
	@docker push quay.io/tuxera/moosefs-master:latest
	@echo "==> Your image is now available at quay.io/tuxera/moosefs-master:$(VERSION)/latest"

push-mfs-chunk:
	@echo "==> Building the quay.io/tuxera/moosefs-chunk docker image"
	@docker build -t quay.io/tuxera/moosefs-chunk:$(VERSION) -f moosefs-chunk.Dockerfile .
	@docker build -t quay.io/tuxera/moosefs-chunk:latest -f moosefs-chunk.Dockerfile .
	@echo "==> Publishing quay.io/tuxera/moosefs-chunk:$(VERSION)"
	@docker push quay.io/tuxera/moosefs-chunk:$(VERSION)
	@docker push quay.io/tuxera/moosefs-chunk:latest
	@echo "==> Your image is now available at quay.io/tuxera/moosefs-chunk:$(VERSION)/latest"

push-mfs-client:
	@echo "==> Building the quay.io/tuxera/moosefs-client docker image"
	@docker build -t quay.io/tuxera/moosefs-client:$(VERSION) -f moosefs-client.Dockerfile .
	@docker build -t quay.io/tuxera/moosefs-client:latest -f moosefs-client.Dockerfile .
	@echo "==> Publishing quay.io/tuxera/moosefs-client:$(VERSION)"
	@docker push quay.io/tuxera/moosefs-client:$(VERSION)
	@docker push quay.io/tuxera/moosefs-client:latest
	@echo "==> Your image is now available at quay.io/tuxera/moosefs-client:$(VERSION)/latest"
