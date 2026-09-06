#!/bin/bash
# run.sh — 工具智能 A/B bench 运行脚本
# 使用 sensenova 配置（默认）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

OUT_DIR="${OUT_DIR:-$PROJECT_ROOT/bench/results/intelligence}"
LIMIT="${LIMIT:-0}"
MODEL="${MODEL:-sensenova-6.8-flash-lite}"
BASE_URL="${BASE_URL:-https://token.sensenova.cn/v1}"
API_KEY="${OPENAI_API_KEY:-}"

if [ -z "$API_KEY" ]; then
  echo "错误: 需要设置 OPENAI_API_KEY 环境变量"
  exit 1
fi

echo "===== 工具智能 A/B Bench ====="
echo "输出目录: $OUT_DIR"
echo "模型: $MODEL"
echo "题面限制: $LIMIT (0=全部)"
echo ""

mkdir -p "$OUT_DIR"

cd "$PROJECT_ROOT"
go run ./bench/intelligence \
  --model "$MODEL" \
  --base-url "$BASE_URL" \
  --api-key "$API_KEY" \
  --out "$OUT_DIR" \
  --limit "$LIMIT"

echo ""
echo "===== Bench 完成 ====="
echo "结果文件: $OUT_DIR/results.jsonl"
