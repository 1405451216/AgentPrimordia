#!/bin/bash
# eval-real 真实轨运行 —— sensenova 网关配置（2026-09-05 切换，moma 额度耗尽）
# 用法: bash run.sh [--mode external|judge] [--set 题面文件] [--holdout] [--limit N]
set -euo pipefail

cd "$(dirname "$0")/../.."

go run ./bench/eval-real \
  --provider openai \
  --model "sensenova-6.8-flash-lite" \
  --base-url "https://token.sensenova.cn/v1" \
  --api-key "sk-IE84nyrP9MdJXleKAfAKGNiawk81sHZW" \
  "$@"
