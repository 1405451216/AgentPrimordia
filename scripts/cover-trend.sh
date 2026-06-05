#!/bin/bash
# cover-trend: 跑当前覆盖率,追加到 docs/coverage-history.md
# 用法: make cover-trend
set -e

date=$(date -u +"%Y-%m-%d")
output=$(go test -cover ./internal/... ./pkg/... -count=1 2>&1 | grep "coverage:" | grep -v "no statements" | sort)

# 追加 Markdown 表格
{
  echo "## $date"
  echo ""
  echo '| Package | Coverage |'
  echo '|---------|--------:|'
  echo "$output" | sed -E 's|.*agentprimordia/([^ ]+) +.*coverage: ([0-9.]+)%.*|\| `\1` | \2% |'
  echo ""
} >> ../docs/coverage-history.md

echo "Appended $date coverage to docs/coverage-history.md"
