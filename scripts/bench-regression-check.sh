#!/usr/bin/env bash
# bench-regression-check.sh — Compare benchmark output against baseline
# Usage: bash scripts/bench-regression-check.sh <bench-output-file> [baseline-json]
#
# Exit codes:
#   0 - All within thresholds
#   1 - Regression detected (>20% deviation) or script error
#
# Thresholds:
#   >10% deviation → WARNING (printed but passes)
#   >20% deviation → FAIL (exit 1)
#
# v4.0 更新：默认基线从 2026-Q2 切换到 2026-Q4（v4.0 性能大版本），
# 并新增关键路径 P95/P99 回归基准（p95_latency_test.go）。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BASELINE_JSON="${2:-$ROOT_DIR/agentprimordia/bench/results/2026-Q4.json}"

WARN_THRESHOLD=10
FAIL_THRESHOLD=20

# --- Baseline values (extracted from baseline JSON) ---
# Format: "benchmark_pattern:baseline_ns_op"
# 数值取自对应基线文件（默认 2026-Q4.json v4.0.0）。
declare -A BASELINES=(
  # cluster: ConsistentHash GetNode
  ["BenchmarkConsistentHash_GetNode/Nodes_10"]=20.18
  ["BenchmarkConsistentHash_GetNode/Nodes_50"]=26.53
  # cluster: DistributedState SetGet sub-benchmarks
  ["BenchmarkDistributedState_SetGet/Set"]=96.32
  ["BenchmarkDistributedState_SetGet/Get"]=53.03
  # capacity: AgentRun
  ["BenchmarkAgentRun"]=3387
  # capacity: MemoryStore Search & Add
  ["BenchmarkMemoryStore/Search"]=28626
  ["BenchmarkMemoryStore/Add"]=49.68
  # v4.0 关键路径 P95（p95_latency_test.go，最慢 5% 批次的平均延迟）
  # v4.2-3 刷新（2026-08-09，v4.1 真实接线落地后 3 次运行中位数）
  ["BenchmarkP95AgentRun"]=2981
  ["BenchmarkP95AgentRun/p95_ns/op"]=10954
  ["BenchmarkP95ToolCall"]=5104
  ["BenchmarkP95ToolCall/p95_ns/op"]=15031
  ["BenchmarkP95MemorySearch"]=43889
  ["BenchmarkP95MemorySearch/p95_ns/op"]=58123
  # v4.2-3 新增：FailureStore SQLite 三条路径 + 内存对照（failure_store_bench_test.go）
  ["BenchmarkP95FailureSQLite_Record"]=6891842
  ["BenchmarkP95FailureSQLite_Get"]=23663
  ["BenchmarkP95FailureSQLite_Get/p95_ns/op"]=30745
  ["BenchmarkP95FailureSQLite_List"]=41906
  ["BenchmarkP95FailureSQLite_List/p95_ns/op"]=51085
  ["BenchmarkP95FailureMemory_Record"]=32
)

# Human-readable labels for reporting
declare -A LABELS=(
  ["BenchmarkConsistentHash_GetNode/Nodes_10"]="ConsistentHash GetNode(10)"
  ["BenchmarkConsistentHash_GetNode/Nodes_50"]="ConsistentHash GetNode(50)"
  ["BenchmarkDistributedState_SetGet/Set"]="DistributedState Set"
  ["BenchmarkDistributedState_SetGet/Get"]="DistributedState Get"
  ["BenchmarkAgentRun"]="AgentRun"
  ["BenchmarkMemoryStore/Search"]="MemoryStore Search"
  ["BenchmarkMemoryStore/Add"]="MemoryStore Add"
  ["BenchmarkP95AgentRun"]="AgentRun P50"
  ["BenchmarkP95AgentRun/p95_ns/op"]="AgentRun P95"
  ["BenchmarkP95ToolCall"]="ToolCall P50"
  ["BenchmarkP95ToolCall/p95_ns/op"]="ToolCall P95"
  ["BenchmarkP95MemorySearch"]="MemorySearch P50"
  ["BenchmarkP95MemorySearch/p95_ns/op"]="MemorySearch P95"
  ["BenchmarkP95FailureSQLite_Record"]="FailureSQLite Record"
  ["BenchmarkP95FailureSQLite_Get"]="FailureSQLite Get P50"
  ["BenchmarkP95FailureSQLite_Get/p95_ns/op"]="FailureSQLite Get P95"
  ["BenchmarkP95FailureSQLite_List"]="FailureSQLite List P50"
  ["BenchmarkP95FailureSQLite_List/p95_ns/op"]="FailureSQLite List P95"
  ["BenchmarkP95FailureMemory_Record"]="FailureMemory Record"
)

# --- Validate inputs ---
if [ $# -lt 1 ]; then
  echo "Usage: $0 <bench-output-file>"
  echo "  bench-output-file: path to file containing 'go test -bench' output"
  exit 1
fi

BENCH_FILE="$1"

if [ ! -f "$BENCH_FILE" ]; then
  echo "ERROR: Benchmark output file not found: $BENCH_FILE"
  exit 1
fi

if [ ! -f "$BASELINE_JSON" ]; then
  echo "ERROR: Baseline JSON not found: $BASELINE_JSON"
  exit 1
fi

echo "=============================================="
echo " Performance Regression Check"
echo "=============================================="
echo "Baseline: $BASELINE_JSON"
echo "Input:    $BENCH_FILE"
echo "Warn:     >${WARN_THRESHOLD}% deviation"
echo "Fail:     >${FAIL_THRESHOLD}% deviation"
echo "----------------------------------------------"
echo ""

# --- Parse benchmark output ---
# Go benchmark output lines look like:
#   BenchmarkConsistentHash_GetNode/Nodes_10-8   50000000   26.1 ns/op   ...
#   BenchmarkP95AgentRun-8   70323   3062 ns/op   3061 p50_ns/op   10765 p95_ns/op ...
# We extract: benchmark_name → ns/op value (main), plus p95_ns/op where present.
declare -A CURRENT=()

while IFS= read -r line; do
  # Match lines with ns/op
  if echo "$line" | grep -q 'ns/op'; then
    # Extract benchmark name (first field, strip CPU count suffix like -8)
    bench_name=$(echo "$line" | awk '{print $1}' | sed 's/-[0-9]*$//')
    # Extract ns/op value (the number before "ns/op")
    ns_val=$(echo "$line" | grep -oP '[0-9]+(\.[0-9]+)?\s+ns/op' | awk '{print $1}')

    if [ -n "$ns_val" ]; then
      CURRENT["$bench_name"]="$ns_val"
    fi

    # v4.0：额外提取 p95_ns/op（若存在）
    if echo "$line" | grep -q 'p95_ns/op'; then
      p95_val=$(echo "$line" | grep -oP '[0-9]+(\.[0-9]+)?\s+p95_ns/op' | awk '{print $1}')
      if [ -n "$p95_val" ]; then
        CURRENT["$bench_name/p95_ns/op"]="$p95_val"
      fi
    fi
  fi
done < "$BENCH_FILE"

if [ ${#CURRENT[@]} -eq 0 ]; then
  echo "ERROR: No benchmark results (ns/op) found in $BENCH_FILE"
  exit 1
fi

echo "Parsed ${#CURRENT[@]} benchmark results from output."
echo ""

# --- Compare against baselines ---
WARNINGS=0
FAILURES=0
PASSED=0

printf "%-40s %12s %12s %10s %s\n" "Benchmark" "Baseline" "Current" "Deviation" "Status"
printf "%-40s %12s %12s %10s %s\n" "----------------------------------------" "------------" "------------" "----------" "--------"

for key in "${!BASELINES[@]}"; do
  baseline="${BASELINES[$key]}"
  label="${LABELS[$key]:-$key}"

  # Find matching current result：先精确匹配，再前缀匹配
  # （避免父 key 因迭代顺序误命中子 key，如 BenchmarkP95AgentRun 误取 p95 子项）
  current_val=""
  if [ -n "${CURRENT[$key]:-}" ]; then
    current_val="${CURRENT[$key]}"
  else
    for bench_key in "${!CURRENT[@]}"; do
      if [[ "$bench_key" == "$key/"* ]]; then
        current_val="${CURRENT[$bench_key]}"
        break
      fi
    done
  fi

  if [ -z "$current_val" ]; then
    printf "%-40s %12s %12s %10s %s\n" "$label" "$baseline" "N/A" "-" "MISSING"
    WARNINGS=$((WARNINGS + 1))
    continue
  fi

  # Calculate deviation percentage: |current - baseline| / baseline * 100
  deviation=$(awk "BEGIN {
    diff = $current_val - $baseline;
    if (diff < 0) diff = -diff;
    pct = (diff / $baseline) * 100;
    printf \"%.1f\", pct
  }")

  # Determine status
  is_higher=$(awk "BEGIN { print ($current_val > $baseline) ? 1 : 0 }")
  direction="faster"
  if [ "$is_higher" = "1" ]; then
    direction="slower"
  fi

  status="PASS"
  dev_int=$(awk "BEGIN { printf \"%d\", $deviation }")

  if [ "$dev_int" -ge "$FAIL_THRESHOLD" ]; then
    status="FAIL"
    FAILURES=$((FAILURES + 1))
  elif [ "$dev_int" -ge "$WARN_THRESHOLD" ]; then
    status="WARN"
    WARNINGS=$((WARNINGS + 1))
  else
    PASSED=$((PASSED + 1))
  fi

  printf "%-40s %12s %12s %9s%% %s (%s)\n" "$label" "$baseline" "$current_val" "$deviation" "$status" "$direction"
done

echo ""
echo "----------------------------------------------"
echo "Summary: $PASSED passed, $WARNINGS warnings, $FAILURES failures"
echo "----------------------------------------------"

if [ "$FAILURES" -gt 0 ]; then
  echo ""
  echo "REGRESSION DETECTED: $FAILURES benchmark(s) exceeded ${FAIL_THRESHOLD}% threshold."
  echo "See details above for which benchmarks regressed."
  exit 1
fi

if [ "$WARNINGS" -gt 0 ]; then
  echo ""
  echo "WARNING: $WARNINGS benchmark(s) exceeded ${WARN_THRESHOLD}% threshold but within ${FAIL_THRESHOLD}%."
  echo "Monitor these for further degradation."
fi

echo ""
echo "All benchmarks within acceptable thresholds."
exit 0
