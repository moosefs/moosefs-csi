VERSION ?= dev
NAME=moosefs-csi-plugin

all: publish

publish: compile build push clean

compile:
	@echo "==> Building the project"
	@env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ${NAME}

build:
	@echo "==> Building the docker image"
	@docker build -t quay.io/tuxera/moosefs-csi-plugin:$(VERSION) .
	@docker build -t quay.io/tuxera/moosefs-csi-plugin:latest .

push:
	@echo "==> Publishing tuxera/do-csi-plugin:$(VERSION)"
	@docker push quay.io/tuxera/moosefs-csi-plugin:$(VERSION)
	@docker push quay.io/tuxera/moosefs-csi-plugin:latest
	@echo "==> Your image is now available at quay.io/tuxera/moosefs-csi-plugin:$(VERSION)/latest"

clean:
	@echo "==> Cleaning releases"
	@GOOS=linux go clean -i -x ./...

.PHONY: all push fetch build-image clean
