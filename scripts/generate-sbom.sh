#!/bin/bash
# generate-sbom.sh — 生成项目 SBOM（CycloneDX / SPDX 格式）
#
# 供 .github/workflows/supply-chain.yml 的 Generate SBOM 步骤使用。
#
# 用法：
#   ./scripts/generate-sbom.sh <output-file> <format>
#     output-file: 输出文件路径（默认 sbom.json）
#     format:      cyclonedx-json | spdx-json（默认 cyclonedx-json）
#
# 依赖：syft（https://github.com/anchore/syft）
set -euo pipefail

OUTPUT="${1:-sbom.json}"
FORMAT="${2:-cyclonedx-json}"

if ! command -v syft &> /dev/null; then
  echo "错误: 未找到 syft。请先安装："
  echo "  curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin"
  exit 1
fi

# 扫描仓库根目录（Go workspace 多模块 + TS SDK 依赖），生成指定格式 SBOM
echo "使用 syft 生成 SBOM（format=${FORMAT}）..."
syft scan dir:. --output "${FORMAT}=${OUTPUT}"

echo "SBOM 已生成: ${OUTPUT}"
