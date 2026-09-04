#!/bin/bash
# v6.1 命题1 世界模型配对AB —— moma 网关配置
# 用法: bash run.sh [--limit N] [--rounds N] [--arms A,B]
set -euo pipefail

cd "$(dirname "$0")/../.."

go run ./bench/eval-v61 \
  --provider openai \
  --model "Deepseek-V4-Flash" \
  --base-url "https://moma.cmecloud.cn/tokenplan-personal/v1" \
  --api-key "mXe-nyFgEhMduWhXKyC9ymkvntYBze_udv1_Ihn0UoY" \
  "$@"
