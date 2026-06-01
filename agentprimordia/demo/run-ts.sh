#!/bin/sh
set -e

echo "=========================================="
echo "  AgentPrimordia — TypeScript Demo"
echo "=========================================="
echo ""

cd /opt/ap-sdk

echo ">>> Running TypeScript SDK demo"
echo "------------------------------------------"
npx tsx /opt/ap-demo/ts-basic.ts
echo ""

echo "=========================================="
echo "  TypeScript demo complete!"
echo "=========================================="
