# gl:docs/dev/go-redir-svc.md#L10
IMAGE_NAME=ghcr.io/mj41/go-redir-svc
TAG=latest

.PHONY: build-image push-image run-local test

build-image:
	podman build -t $(IMAGE_NAME):$(TAG) .

push-image:
	podman push $(IMAGE_NAME):$(TAG)

run-local:
	go run main.go --config config.yaml --log-dir ./logs

test:
	go test -v ./...
