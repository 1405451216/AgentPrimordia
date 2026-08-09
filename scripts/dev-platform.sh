#!/usr/bin/env bash
# ============================================================
# AgentPrimordia Platform - one-command dev platform (macOS / Linux)
#
# v5.0-1 一体化一键部署（开发形态）：拉起全部能力入口
#   - Studio 后端（:8090，真实引擎注入模式）
#   - 各能力演示（autonomy / skills / realtime / a2a 示例编译验证）
# 生产形态见 deploy/helm + deploy/terraform（企业部署指南）。
#
# Usage:
#   ./scripts/dev-platform.sh
# ============================================================

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENT_DIR="$ROOT/agentprimordia"
PORT="${AP_PLATFORM_PORT:-8090}"

if ! command -v go >/dev/null 2>&1; then
  echo "error: missing command: go (install Go 1.26+ first)" >&2
  exit 1
fi

echo "==> AgentPrimordia Platform launcher (dev 形态)"
echo "    Studio backend: :$PORT"

# 1. 验证全能力示例可编译（四跃迁 + 双 SDK 契约）
echo "==> 验证全能力示例编译..."
cd "$AGENT_DIR"
for example in autonomous-task skill-evolution realtime-voice a2a-interop coding-agent; do
  go build ./ecosystem/examples/$example/
  echo "    ✓ $example"
done
echo "    ✓ 跨语言契约: $(cd "$ROOT" && node scripts/cross-language-api-check.mjs >/dev/null && echo PASS)"

# 2. 启动 Studio 后端（真实引擎注入模式）
echo "==> 启动 Studio 后端 :$PORT ..."
cd "$AGENT_DIR"
go run ./cmd/studio -addr ":$PORT"
