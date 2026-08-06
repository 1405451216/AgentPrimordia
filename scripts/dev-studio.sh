#!/usr/bin/env bash
# ============================================================
# AgentPrimordia Studio — 一键启动（macOS / Linux）
#
# 同时启动后端（:8090）与前端（:5173），Ctrl+C 一起停止，
# 启动完成后自动打开浏览器。
#
# 用法：
#   ./scripts/dev-studio.sh            # 默认端口
#   ./scripts/dev-studio.sh -p 9000    # 自定义后端端口
# ============================================================

set -euo pipefail

BACKEND_PORT=8090
FRONTEND_PORT=5173
NO_BROWSER=0

while getopts "b:f:h" opt; do
  case "$opt" in
    b) BACKEND_PORT="$OPTARG" ;;
    f) FRONTEND_PORT="$OPTARG" ;;
    h) echo "用法: $0 [-b 后端端口] [-f 前端端口]"; exit 0 ;;
    *) exit 1 ;;
  esac
done

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_DIR="$ROOT/agentprimordia"
FRONTEND_DIR="$ROOT/agentprimordia/studio/web"

echo "==> AgentPrimordia Studio 启动器"
echo "    后端 :$BACKEND_PORT (go run ./cmd/studio)"
echo "    前端 :$FRONTEND_PORT (npm run dev)"

command -v go >/dev/null || { echo "缺少依赖：go" >&2; exit 1; }
command -v npm >/dev/null || { echo "缺少依赖：npm" >&2; exit 1; }

if [ ! -d "$FRONTEND_DIR/node_modules" ]; then
  echo "==> 首次运行：安装前端依赖..."
  (cd "$FRONTEND_DIR" && npm install --no-audit --no-fund)
fi

cleanup() {
  echo ""
  echo "正在停止... $BACKEND_PID $FRONTEND_PID"
  kill "$BACKEND_PID" "$FRONTEND_PID" 2>/dev/null || true
}
trap cleanup INT TERM EXIT

echo "==> 启动后端..."
(cd "$BACKEND_DIR" && go run ./cmd/studio -addr ":$BACKEND_PORT") &
BACKEND_PID=$!

echo "==> 启动前端..."
(cd "$FRONTEND_DIR" && npm run dev -- --port "$FRONTEND_PORT" --strictPort) &
FRONTEND_PID=$!

echo ""
echo "==> 等待服务就绪..."
sleep 3
URL="http://localhost:$FRONTEND_PORT"
echo "==> 打开 $URL"
if [ "$NO_BROWSER" = "0" ]; then
  { command -v xdg-open >/dev/null && xdg-open "$URL"; } \
    || { command -v open >/dev/null && open "$URL"; } \
    || true
fi

wait
