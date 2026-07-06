# {{.ProjectName}} Makefile

PLUGIN_NAME := {{.ProjectName}}
VERSION ?= 0.1.0
COSIGN_KEY ?= $(HOME)/.cosign/cosign.key

.PHONY: all build test lint sign publish clean

all: build

## build: 编译插件为 .so（Go plugin 模式）
build:
	go build -o $(PLUGIN_NAME).so -buildmode=plugin .

## test: 运行单元测试
test:
	go test -race -coverprofile=coverage.out ./...

## lint: 静态检查
lint:
	go vet ./...
	gofmt -l .

## sign: 用 cosign 对插件包签名
sign:
	cosign sign-blob --key $(COSIGN_KEY) $(PLUGIN_NAME).so

## verify: 验证签名（CI 阶段使用）
verify:
	cosign verify-blob --key $(COSIGN_KEY) $(PLUGIN_NAME).so

## publish: 发布到 AgentPrimordia 插件注册中心
publish:
	ap plugin publish ./$(PLUGIN_NAME).so \
		--name $(PLUGIN_NAME) \
		--version $(VERSION) \
		--description "{{.ProjectName}} plugin"

## clean: 清理构建产物
clean:
	rm -f $(PLUGIN_NAME).so coverage.out