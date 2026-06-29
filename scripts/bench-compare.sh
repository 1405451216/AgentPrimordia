#!/usr/bin/env bash
# bench-compare.sh — 同时运行 Go 和 TypeScript SDK 的性能基准，生成对比报告
# 用法: bash scripts/bench-compare.sh [--output report.md]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
GO_SDK_DIR="$ROOT_DIR/agentprimordia"
TS_SDK_DIR="$ROOT_DIR/sdk/typescript"
OUTPUT_FILE="${1:-$ROOT_DIR/docs/benchmarks/latest-report.md}"

echo "=== AgentPrimordia Go vs TypeScript 性能基准对比 ==="
echo ""

# 1. Go SDK 基准
echo "[1/2] 运行 Go SDK 基准..."
cd "$GO_SDK_DIR"

GO_BENCH_FILE=$(mktemp /tmp/go-bench-XXXXXX.txt)
go test -bench=. -benchmem -benchtime=10x -count=3 ./bench/suite/... > "$GO_BENCH_FILE" 2>&1 || {
    echo "警告: Go 基准测试部分失败"
}
echo "Go 基准结果: $GO_BENCH_FILE"

# 2. TypeScript SDK 基准
echo "[2/2] 运行 TypeScript SDK 基准..."
cd "$TS_SDK_DIR"

# 确保 dist/ 已编译
npm run build 2>/dev/null || true

TS_BENCH_FILE=$(mktemp /tmp/ts-bench-XXXXXX.txt)
{
    echo "=== TypeScript SDK 基准测试 ==="
    for bench_file in bench/*.bench.js; do
        if [ -f "$bench_file" ]; then
            echo ""
            echo "--- $(basename $bench_file) ---"
            node "$bench_file" 2>&1 || echo "  (失败)"
        fi
    done
} > "$TS_BENCH_FILE"
echo "TS 基准结果: $TS_BENCH_FILE"

# 3. 生成报告
echo ""
echo "=== 生成对比报告 ==="

mkdir -p "$(dirname "$OUTPUT_FILE")"

cat > "$OUTPUT_FILE" << EOF
# 性能基准对比报告

> 生成时间: $(date -u '+%Y-%m-%d %H:%M:%S UTC')
> 环境: $(uname -srm)
> Go: $(go version)
> Node: $(node --version)

## Go SDK 基准

\`\`\`
$(cat "$GO_BENCH_FILE")
\`\`\`

## TypeScript SDK 基准

\`\`\`
$(cat "$TS_BENCH_FILE")
\`\`\`

## 对比分析

详见 [Go vs TypeScript 性能基准对比文档](./go-vs-typescript.md)。
EOF

echo "报告已生成: $OUTPUT_FILE"

# 清理
rm -f "$GO_BENCH_FILE" "$TS_BENCH_FILE"

echo ""
echo "=== 完成 ==="
