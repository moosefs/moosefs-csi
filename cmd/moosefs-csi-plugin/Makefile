VERSION ?= dev
NAME=moosefs-csi-plugin

all: ready

ready: clean cred test compile

publish: build push-image

cred:
	@echo "==> Scanning secrets in commit history (prevent accidents)"
	trufflehog --regex --entropy=False file:///$(shell pwd)/../../

compile:
	@echo "==> Building the project"
	@env CGO_ENABLED=0 GOCACHE=/tmp/go-cache GOOS=linux GOARCH=amd64 go build -a -o ${NAME}

test:
	@echo "==> Running tests"
	go test ../../driver/*

build:
	@echo "==> Building the docker image"
	@docker build -t quay.io/tuxera/moosefs-csi-plugin:$(VERSION) .
	@docker build -t quay.io/tuxera/moosefs-csi-plugin:latest .

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
