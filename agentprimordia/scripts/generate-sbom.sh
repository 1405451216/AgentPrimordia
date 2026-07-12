#!/bin/bash
# scripts/generate-sbom.sh
#
# 生成 CycloneDX 格式的 SBOM（Software Bill of Materials）
# 依赖 syft (https://github.com/anchore/syft)
#
# 用法：
#   ./scripts/generate-sbom.sh [output-path] [format]
# 示例：
#   ./scripts/generate-sbom.sh sbom.json cyclonedx-json
#   ./scripts/generate-sbom.sh sbom.spdx spdx-json

set -euo pipefail

OUTPUT_PATH="${1:-sbom.json}"
FORMAT="${2:-cyclonedx-json}"
CONTEXT_DIR="${3:-.}"

echo "[SBOM] Generating $FORMAT SBOM for $CONTEXT_DIR ..."

if ! command -v syft &> /dev/null; then
    echo "[SBOM] ERROR: syft not found. Install from https://github.com/anchore/syft"
    exit 1
fi

syft packages "dir:$CONTEXT_DIR" -o "$FORMAT" > "$OUTPUT_PATH"

# 验证输出文件
if [ ! -s "$OUTPUT_PATH" ]; then
    echo "[SBOM] ERROR: Generated SBOM is empty"
    exit 1
fi

echo "[SBOM] Successfully generated: $OUTPUT_PATH"

# 可选：验证 SBOM 格式（如有 jq）
if command -v jq &> /dev/null && [[ "$FORMAT" == *"json"* ]]; then
    if jq empty "$OUTPUT_PATH" 2>/dev/null; then
        echo "[SBOM] JSON validation passed"
    else
        echo "[SBOM] WARNING: JSON validation failed"
        exit 1
    fi
fi
