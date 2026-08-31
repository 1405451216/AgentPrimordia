.PHONY: build test test-race test-short test-integration test-distributed-backends lint lint-all lint-go lint-ts cover cover-html cover-check \
        deprecation-check benchmark api-diff api-extract cover-trend clean run-hello run-multi run-production \
        docker-build docker-run docker-clean \
        pgo-profile pgo-build pgo-clean test-ts test-affected

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -ldflags="-X main.Version=$(VERSION)"

# 项目强制无 CGO（modernc.org/sqlite 为纯 Go 驱动），所有目标默认继承此设置。
export CGO_ENABLED=0

# PGO（Profile-Guided Optimization）配置
# Go 1.21+ 支持 PGO，可通过代表性工作负载 profile 指导编译器优化（通常 2-7% 提速）
# 详见 docs/advanced/pgo.md
PGO_PROFILE ?= default.pgo
PGO_DIR ?= .pgo

build:
	go build $(LDFLAGS) -o ap ./cmd/ap/

test:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# race 检测需要 CGO，仅在已安装 C 编译器的环境显式调用：
#   make test-race CGO_ENABLED=1
# Windows 默认无 gcc，请勿在此平台启用。
test-race:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 跨平台友好的测试（无 -race，Windows 无 gcc 也能跑）
test-short:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 集成测试（需要 OPENAI_API_KEY 等环境变量）
test-integration:
	go test -tags=integration -timeout 5m -v ./...

# 分布式后端集成测试（需要真实 etcd + Redis）
# 先启动依赖：docker compose -f deploy/compose/distributed-test.yaml up -d
# 无服务时测试会优雅跳过（skip），有服务时跑真实 CRUD/租约/发现链路
test-distributed-backends:
	go test -tags=etcd,redis -count=1 -timeout 10m -v ./internal/persist/...
	go test -tags=etcd -count=1 -timeout 10m -v ./internal/agent/cluster/...

# 跑覆盖率检查并打印报告
cover:
	go test -coverprofile=coverage.out ./internal/... ./pkg/... 2>&1 | tee coverage.txt
	@echo ""
	@echo "=== 覆盖率汇总 ==="
	@grep "coverage:" coverage.txt | sort

# 生成 HTML 报告（需先跑 cover）
cover-html:
	go tool cover -html=coverage.out -o coverage.html
	@echo "已生成 coverage.html，浏览器打开查看"

# 覆盖率门槛检查（Phase 7.2 引入）
cover-check:
	@go test -coverprofile=/tmp/cover-check.out ./internal/... ./pkg/... > /dev/null 2>&1
	@echo "=== 覆盖率门槛检查 ==="
	@fail=0
	@for pkg in internal/agent internal/llm pkg; do \
		cov=$$(go test -cover ./$$pkg 2>&1 | grep -oE '[0-9.]+%' | head -1 | tr -d '%'); \
		if [ -z "$$cov" ]; then echo "  $$pkg: SKIP (no test)"; continue; fi; \
		echo "  $$pkg: $$cov%"; \
		awk -v c=$$cov 'BEGIN { if (c < 65) exit 1 }' && continue; \
		fail=1; \
	done
	@if [ $$fail -eq 1 ]; then \
		echo ""; \
		echo "FAIL: 核心包 (internal/llm, pkg) 覆盖率 < 65%"; \
		exit 1; \
	fi
	@echo ""
	@echo "OK: 核心包覆盖率门槛通过"

# Phase 12.1: API 变化对比
api-diff:
	bash ../scripts/api-diff.sh $(REF)

# API 契约提取：从 pkg/ 提取公共 API 签名，输出 JSON 供跨语言漂移检测
api-extract:
	go run ./scripts/api-extract/ -output ../sdk/typescript/api-contract.json -no-timestamp

# Phase 12.2: 覆盖率趋势跟踪
cover-trend:
	bash ../scripts/cover-trend.sh

# Phase 12.3: Deprecated 标注完整性检查
deprecation-check:
	bash ../scripts/deprecation-check.sh

# Phase 12.4: 性能基准
benchmark:
	go test -bench=. -benchmem -benchtime=5x -count=3 ./bench/suite/... > bench-output.txt
	@echo "Benchmark results: bench-output.txt"

# ===== PGO（Profile-Guided Optimization）=====
# Go 1.21+ 的 PGO 流程：
# 1. go test -cpuprofile 生成 CPU profile（代表性工作负载）
# 2. 将 profile 重命名为 default.pgo 放入主包目录
# 3. go build -pgo=auto 自动使用该 profile 指导编译
# 预期收益：2-7% 性能提升（取决于工作负载特征）
# 详见 https://go.dev/doc/pgo

# pgo-profile: 运行代表性 benchmark 生成 CPU profile
pgo-profile:
	@echo "=== 生成 PGO Profile ==="
	@mkdir -p $(PGO_DIR)
	go test -run=^$$ -bench=. -benchmem -benchtime=10x -cpuprofile=$(PGO_DIR)/cpu.prof ./bench/suite/...
	@echo "Profile 已生成: $(PGO_DIR)/cpu.prof"
	@echo "运行 'make pgo-build' 使用此 profile 构建优化二进制"

# pgo-build: 使用 PGO profile 构建优化的二进制文件
pgo-build:
	@echo "=== PGO 构建 ==="
	@if [ ! -f "$(PGO_DIR)/cpu.prof" ]; then \
		echo "错误: 未找到 PGO profile ($(PGO_DIR)/cpu.prof)"; \
		echo "请先运行 'make pgo-profile'"; \
		exit 1; \
	fi
	cp $(PGO_DIR)/cpu.prof $(PGO_PROFILE)
	go build $(LDFLAGS) -pgo=$(PGO_PROFILE) -o ap ./cmd/ap/
	@echo "PGO 优化二进制已构建: ap"
	@echo "清理: make pgo-clean"

# pgo-clean: 清理 PGO 临时文件
pgo-clean:
	rm -f $(PGO_PROFILE)
	rm -rf $(PGO_DIR)
	@echo "PGO 临时文件已清理"

# Phase 12.5: 统一 lint 入口
lint-all: lint-go lint-ts
	@echo "All lint checks passed"

lint-go:
	golangci-lint run ./...

lint-ts:
	cd ../sdk/typescript && npm run lint
	@echo "=== TS typecheck (tsc --noEmit) ==="
	cd ../sdk/typescript && npx tsc --noEmit

# 统一 TS SDK 测试入口
# sdk/typescript 使用 vitest（直接测试 TS 源码）
test-ts:
	@echo "=== TS SDK 测试 ==="
	cd ../sdk/typescript && npx vitest run --coverage

lint:
	golangci-lint run ./...

clean:
	rm -rf coverage.out coverage.html coverage.txt dist/ bench-output.txt

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

# 按变更范围运行测试：仅对 git diff 涉及的 Go 包执行 go test
# 用法: make test-affected [BASE=origin/main]
BASE ?= origin/main
test-affected:
	@echo "=== 按变更范围运行 Go 测试 (base: $(BASE)) ==="
	@pkgs=$$(git diff --name-only $(BASE) -- '*.go' | grep -v '_test.go' | xargs -r -n1 dirname | sort -u | sed 's|^agentprimordia/||' | sed 's|^|./|'); \
	if [ -z "$$pkgs" ]; then \
		echo "无 Go 源码变更，跳过测试"; \
	else \
		echo "受影响包:"; \
		echo "$$pkgs" | tr ' ' '\n'; \
		echo ""; \
		go test -v -count=1 $$pkgs; \
	fi

# Protocol Buffers generation
.PHONY: proto
proto:
	@mkdir -p internal/agent/a2a/proto/a2a/v1
	protoc \
		--go_out=. \
		--go_opt=module=agentprimordia \
		--go-grpc_out=. \
		--go-grpc_opt=module=agentprimordia \
		internal/agent/a2a/proto/a2a/v1/a2a.proto
