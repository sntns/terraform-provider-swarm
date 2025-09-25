BINARY_NAME=terraform-provider-swarm
VERSION=0.1.0

.PHONY: build test clean install fmt

build:
	go build -o $(BINARY_NAME) -ldflags "-X main.version=$(VERSION)"

test:
	go test ./...

fmt:
	go fmt ./...

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

install: build
	mkdir -p ~/.terraform.d/plugins/sntns/swarm/$(VERSION)/linux_amd64/
	cp $(BINARY_NAME) ~/.terraform.d/plugins/sntns/swarm/$(VERSION)/linux_amd64/

docs:
	@echo "Documentation is available in README.md and examples/ directory"

.DEFAULT_GOAL := build