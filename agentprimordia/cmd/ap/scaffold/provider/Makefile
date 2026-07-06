# {{.ProjectName}} Makefile

.PHONY: all test lint build clean

all: test

## test: 运行单元测试
test:
	go test -race -coverprofile=coverage.out ./...

## lint: 静态检查
lint:
	go vet ./...
	gofmt -l .

## build: 验证编译
build:
	go build ./...

## clean: 清理构建产物
clean:
	rm -f coverage.out