#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/../.."
go run ./bench/planning-v71 \
  --model "sensenova-6.8-flash-lite" \
  --base-url "https://token.sensenova.cn/v1" \
  --api-key "sk-IE84nyrP9MdJXleKAfAKGNiawk81sHZW" \
  --out bench/results/planning-v71 \
  "$@"
