#!/bin/bash
# deprecation-check: 验证每个 // Deprecated: 都有对应的 // Removed in vX.Y.
#
# 精度说明（2026-08 修正）：
#   - 只统计符号级标注 `^// Deprecated:`，排除文档块中的 `//\tDeprecated:` 提及
#     （如 pkg/agent.go 包注释里对 4 级 Stability 分类的文字描述）
#   - 排除生成代码（*.pb.go）与测试文件（*_test.go），二者不承载 API 废弃策略
#   - 按文件粒度校验：同一文件内 Deprecated 数量必须 <= Removed in 数量
#
# v4.0 强化（v4.0-1 验收）：对 pkg/ 公共 API，额外校验"已承诺移除（Removed in）
# 的 API 不得仍在 pkg 中导出"，实现 deprecation 检查 0 残留。
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

# v4.0-1：pkg/ 公共 API 不允许"已标记 Removed"却仍然导出的符号。
# 简单约定：若某文件同时含 Deprecated 与 Removed in，且该文件位于 pkg/，
# 则要求 Removed 版本必须 >= 当前主版本（v4），否则视为超期残留。
CURRENT_MAJOR=4
pkg_files=$(echo "$files" | grep -E '(^|/)pkg/' || true)
if [ -n "$pkg_files" ]; then
  for f in $pkg_files; do
    # 提取该文件中所有 Removed in vX.Y 的主版本号
    for v in $(grep -oE 'Removed in v[0-9]+' "$f" | grep -oE '[0-9]+' || true); do
      if [ "$v" -lt "$CURRENT_MAJOR" ]; then
        echo "ERROR: $f — 承诺 Removed in v$v（已超期，当前主版本 v$CURRENT_MAJOR），但 API 仍导出，请移除或更新"
        fail=1
      fi
    done
  done
fi

if [ "$fail" -eq 1 ]; then
  echo ""
  echo "每个 // Deprecated: 标注都必须配有 // Removed in vX.Y. 标注；"
  echo "pkg/ 公共 API 的 Removed 版本不得早于当前主版本（v4.0 验收：deprecation 检查 0 残留）"
  exit 1
fi
echo "OK: Deprecated 标注完整"

