.PHONY: build test codegen

build:
	GOWORK=off go build ./...

test:
	GOWORK=off go test ./... -count=1

codegen:
	controller-gen object paths=./api/v1/...
