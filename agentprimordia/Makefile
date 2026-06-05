.PHONY: build test test-short test-integration lint lint-all lint-go lint-ts cover cover-html cover-check \
        deprecation-check benchmark api-diff cover-trend clean run-hello run-multi run-production \
        docker-build docker-run docker-clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -ldflags="-X main.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o ap ./cmd/ap/

test:
	@if go env CGO_ENABLED | grep -q 1 2>/dev/null; then \
		go test -v -race -coverprofile=coverage.out ./...; \
	else \
		echo "CGO disabled, running without -race"; \
		go test -v -coverprofile=coverage.out ./...; \
	fi
	go tool cover -html=coverage.out -o coverage.html

# 跨平台友好的测试（无 -race，Windows 无 gcc 也能跑）
test-short:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 集成测试（需要 OPENAI_API_KEY 等环境变量）
test-integration:
	go test -tags=integration -timeout 5m -v ./...

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

# Phase 12.5: 统一 lint 入口
lint-all: lint-go lint-ts
	@echo "All lint checks passed"

lint-go:
	golangci-lint run ./...

lint-ts:
	cd sdk/typescript && npm run lint

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
