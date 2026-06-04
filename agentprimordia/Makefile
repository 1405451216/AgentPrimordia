.PHONY: build test test-short lint cover cover-html cover-check clean run-hello run-multi run-production docker-build docker-run docker-clean

build:
	go build ./...

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# 跨平台友好的测试（无 -race，Windows 无 gcc 也能跑）
test-short:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

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
# 软门：internal/agent ≥ 80% (核心包)
# 硬门：internal/llm + pkg ≥ 65% (其他核心包)
# 报警：单包 < 50%
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

lint:
	golangci-lint run ./...

clean:
	rm -rf coverage.out coverage.html coverage.txt dist/

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
