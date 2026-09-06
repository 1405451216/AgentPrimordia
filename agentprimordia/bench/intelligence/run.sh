#!/bin/bash
# run.sh — 工具智能 A/B bench 运行脚本
# 使用 sensenova 配置（默认）

set -euo pipefail

OUT_DIR="${OUT_DIR:-bench/results/intelligence}"
LIMIT="${LIMIT:-0}"

echo "===== 工具智能 A/B Bench ====="
echo "输出目录: $OUT_DIR"
echo "题面限制: $LIMIT (0=全部)"
echo ""

# 确保输出目录存在
mkdir -p "$OUT_DIR"

# 运行 bench
go run ./bench/intelligence \
  --out "$OUT_DIR" \
  --limit "$LIMIT"

echo ""
echo "===== Bench 完成 ====="
echo "结果文件: $OUT_DIR/results.jsonl"
