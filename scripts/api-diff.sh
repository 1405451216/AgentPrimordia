#!/bin/bash
# api-diff: 对比 <ref>..HEAD 的 pkg/ export 变化
# 用法: make api-diff REF=origin/main
set -e

REF=${1:-origin/main}

echo "API Diff (since $REF)"
echo ""

current=$(mktemp)
base=$(mktemp)
trap "rm -f $current $base" EXIT

# 收集当前 pkg/ export
(go doc -all ./pkg/...) 2>/dev/null | grep -E "^(func|type|var|const) " | sort > "$current" || true

# 收集基线 pkg/ export
git stash -q 2>/dev/null || true
(git checkout "$REF" -q 2>/dev/null && go doc -all ./pkg/...) 2>/dev/null | grep -E "^(func|type|var|const) " | sort > "$base" || true
git checkout - -q 2>/dev/null || true
git stash pop -q 2>/dev/null || true

echo "Added:"
comm -23 "$current" "$base" | sed 's/^/  + /'

echo ""
echo "Removed:"
comm -13 "$current" "$base" | sed 's/^/  - /'

echo ""
added=$(comm -23 "$current" "$base" | wc -l)
removed=$(comm -13 "$current" "$base" | wc -l)
echo "Summary: +${added} -${removed} exports"
