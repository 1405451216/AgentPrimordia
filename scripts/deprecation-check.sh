#!/bin/bash
# deprecation-check: 验证每个 // Deprecated: 都有对应的 // Removed in vX.Y.
#
# 精度说明（2026-08 修正）：
#   - 只统计符号级标注 `^// Deprecated:`，排除文档块中的 `//\tDeprecated:` 提及
#     （如 pkg/agent.go 包注释里对 4 级 Stability 分类的文字描述）
#   - 排除生成代码（*.pb.go）与测试文件（*_test.go），二者不承载 API 废弃策略
#   - 按文件粒度校验：同一文件内 Deprecated 数量必须 <= Removed in 数量
#
# 用法：bash scripts/deprecation-check.sh

set -e

# 收集含符号级 Deprecated 标注的非生成、非测试 Go 文件
files=$(grep -rl --include="*.go" -e '^// Deprecated:' . 2>/dev/null | grep -vE '(_test\.go|\.pb\.go)$' || true)

if [ -z "$files" ]; then
  echo "OK: 无 Deprecated 标注"
  exit 0
fi

fail=0
total_deprecated=0
total_removed=0

for f in $files; do
  d=$(grep -c -e '^// Deprecated:' "$f" || true)
  r=$(grep -c -e '^// Removed in v' "$f" || true)
  total_deprecated=$((total_deprecated + d))
  total_removed=$((total_removed + r))
  if [ "$r" -lt "$d" ]; then
    echo "ERROR: $f — $d 处 Deprecated，但仅 $r 处 Removed in"
    fail=1
  fi
done

echo "Deprecated: $total_deprecated, Removed in: $total_removed"

if [ "$fail" -eq 1 ]; then
  echo ""
  echo "每个 // Deprecated: 标注都必须配有 // Removed in vX.Y. 标注"
  exit 1
fi
echo "OK: Deprecated 标注完整"
