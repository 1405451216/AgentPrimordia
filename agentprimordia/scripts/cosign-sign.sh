#!/bin/bash
# scripts/cosign-sign.sh
#
# 对容器镜像进行 Cosign 签名
# 依赖 cosign (https://github.com/sigstore/cosign)
#
# 用法：
#   ./scripts/cosign-sign.sh <image-ref>
# 示例：
#   ./scripts/cosign-sign.sh ghcr.io/example/app:latest
#
# 环境变量：
#   COSIGN_PRIVATE_KEY  - 签名私钥（env:// 模式）
#   COSIGN_PASSWORD     - 私钥密码

set -euo pipefail

IMAGE_REF="${1:-}"

if [ -z "$IMAGE_REF" ]; then
    echo "[Cosign] ERROR: Usage: $0 <image-ref>"
    exit 1
fi

echo "[Cosign] Signing image: $IMAGE_REF"

if ! command -v cosign &> /dev/null; then
    echo "[Cosign] ERROR: cosign not found. Install from https://github.com/sigstore/cosign"
    exit 1
fi

# 检查是否使用 keyless 模式（Fulcio + Rekor）
if [ -n "${COSIGN_PRIVATE_KEY:-}" ]; then
    echo "[Cosign] Using key-based signing"
    cosign sign --key env://COSIGN_PRIVATE_KEY "$IMAGE_REF"
else
    echo "[Cosign] Using keyless signing (Fulcio)"
    cosign sign "$IMAGE_REF"
fi

echo "[Cosign] Successfully signed: $IMAGE_REF"

# 验证签名
echo "[Cosign] Verifying signature..."
if cosign verify "$IMAGE_REF" 2>/dev/null; then
    echo "[Cosign] Signature verified"
else
    echo "[Cosign] WARNING: Signature verification skipped or failed"
fi
