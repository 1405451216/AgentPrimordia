#!/usr/bin/env bash
# bench-regression-check.sh — Compare benchmark output against baseline from 2026-Q2.json
# Usage: bash scripts/bench-regression-check.sh <bench-output-file>
#
# Exit codes:
#   0 - All within thresholds
#   1 - Regression detected (>20% deviation) or script error
#
# Thresholds:
#   >10% deviation → WARNING (printed but passes)
#   >20% deviation → FAIL (exit 1)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BASELINE_JSON="$ROOT_DIR/agentprimordia/bench/results/2026-Q2.json"

WARN_THRESHOLD=10
FAIL_THRESHOLD=20

# --- Baseline values (extracted from 2026-Q2.json v31_suites) ---
# Format: "benchmark_pattern:baseline_ns_op"
# These are the authoritative numeric values from the Q2 baseline run.
declare -A BASELINES=(
  # cluster: ConsistentHash GetNode (v31_suites.cluster.details)
  ["BenchmarkConsistentHash_GetNode/Nodes_10"]=25.65
  ["BenchmarkConsistentHash_GetNode/Nodes_50"]=34.31
  # cluster: DistributedState SetGet sub-benchmarks (v31_suites.cluster.details)
  # JSON details: "89.22/52.17 ns/op" → Set=89.22, Get=52.17
  ["BenchmarkDistributedState_SetGet/Set"]=89.22
  ["BenchmarkDistributedState_SetGet/Get"]=52.17
  # capacity: AgentRun (v31_suites.capacity.details)
  ["BenchmarkAgentRun"]=4782
  # capacity: MemoryStore Search & Add (v31_suites.capacity.details)
  ["BenchmarkMemoryStore/Search"]=30019
  ["BenchmarkMemoryStore/Add"]=46.28
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
echo "Baseline: $BASELINE_JSON (2026-Q2, v3.1.0)"
echo "Input:    $BENCH_FILE"
echo "Warn:     >${WARN_THRESHOLD}% deviation"
echo "Fail:     >${FAIL_THRESHOLD}% deviation"
echo "----------------------------------------------"
echo ""

# --- Parse benchmark output ---
# Go benchmark output lines look like:
#   BenchmarkConsistentHash_GetNode/Nodes_10-8   50000000   26.1 ns/op   ...
# We extract: benchmark_name → ns/op value
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

  # Find matching current result (exact or prefix match)
  current_val=""
  for bench_key in "${!CURRENT[@]}"; do
    if [ "$bench_key" = "$key" ] || [[ "$bench_key" == "$key/"* ]]; then
      current_val="${CURRENT[$bench_key]}"
      break
    fi
  done

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
