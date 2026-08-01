#!/bin/bash
# cosign-sign.sh — 使用 cosign 对镜像签名
#
# 供 .github/workflows/supply-chain.yml 的 Sign Image 步骤使用。
# 支持 keyless（COSIGN_EXPERIMENTAL=1）与 key-based（COSIGN_KEY + COSIGN_PASSWORD）两种模式。
#
# 用法：
#   ./scripts/cosign-sign.sh <image-ref>
set -euo pipefail

IMAGE="${1:-}"
if [ -z "$IMAGE" ]; then
  echo "用法: $0 <image-ref>" >&2
  exit 2
fi

if ! command -v cosign &> /dev/null; then
  echo "错误: 未找到 cosign。" >&2
  exit 1
fi

if [ -n "${COSIGN_KEY:-}" ] && [ -n "${COSIGN_PASSWORD:-}" ]; then
  echo "使用 key-based 签名: $IMAGE"
  cosign sign --yes --key env://COSIGN_KEY "$IMAGE"
else
  echo "使用 keyless 签名（COSIGN_EXPERIMENTAL=${COSIGN_EXPERIMENTAL:-0}）: $IMAGE"
  cosign sign --yes "$IMAGE"
fi
