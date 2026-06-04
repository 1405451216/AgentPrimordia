.PHONY: build test lint clean run-hello run-multi run-production docker-build docker-run docker-clean

build:
	go build ./...

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

clean:
	rm -rf coverage.out coverage.html dist/

run-hello:
	go run ./ecosystem/examples/chain-api/

run-multi:
	go run ./ecosystem/examples/multi-agent/

run-production:
	go run ./ecosystem/examples/chain-production/

docker-build:
	docker compose build

docker-run:
	docker compose up

docker-clean:
	docker compose down --rmi local --volumes --remove-orphans
