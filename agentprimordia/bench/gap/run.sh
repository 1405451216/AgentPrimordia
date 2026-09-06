#!/bin/bash
# E-4 缺口工具 A/B 实验
set -euo pipefail
cd "$(dirname "$0")/../.."
go run ./bench/gap \
  --model "sensenova-6.8-flash-lite" \
  --base-url "https://token.sensenova.cn/v1" \
  --api-key "sk-IE84nyrP9MdJXleKAfAKGNiawk81sHZW" \
  --out bench/results/gap \
  "$@"
