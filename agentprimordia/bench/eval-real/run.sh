#!/bin/bash
# eval-real 真实轨运行 —— moma 网关配置
# 用法: bash run.sh [--mode external|judge] [--set 题面文件] [--holdout] [--limit N]
set -euo pipefail

cd "$(dirname "$0")/../.."

go run ./bench/eval-real \
  --provider openai \
  --model "Deepseek-V4-Flash" \
  --base-url "https://moma.cmecloud.cn/tokenplan-personal/v1" \
  --api-key "mXe-nyFgEhMduWhXKyC9ymkvntYBze_udv1_Ihn0UoY" \
  "$@"
