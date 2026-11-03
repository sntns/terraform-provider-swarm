default: lint test

build:
	go build -v ./...

lint:
	golangci-lint run

generate:
	go generate ./...

test:
	go test -race -cover ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 30m ./...

.PHONY: build lint generate test testacc
