# 推理加速与边缘 Profile（v4.9-3）

## 一、云端推理加速（已有能力）

| 策略 | 实现 | 启用 |
|------|------|------|
| LLM 批量合并 | `internal/llm/batch.go` + Pool `SetLLMBatchProcessor` | Pool 装配时注入 |
| 多模型路由 | `internal/llm/model_router.go`（Cost/Quality/Balanced） | RouterProvider 包装 |
| 语义缓存 | L1 内存 + L2 SQLite，`HitRate/TokensSaved` 指标 | `CachedProvider` |
| 重试/熔断/降级 | `ResilientProvider` | 包装主 Provider |

## 二、边缘推理 Profile（浏览器 WebGPU / 本地）

链路（v4.3 真实化）：`WebGPUModelRunner` → `TransformersBackend`（动态导入 @xenova/transformers，WebGPU q4 量化）→ `SkeletonBackend` 仅回退。

### 延迟报告模板

```json
{
  "profile": "edge-webgpu-qwen3-0.5b",
  "environment": { "device": "webgpu", "quant": "q4", "browser": "chrome" },
  "metrics": {
    "model_load_ms": 0,
    "first_token_ms": 0,
    "tokens_per_sec": 0,
    "p50_ms": 0,
    "p95_ms": 0
  },
  "fallback_hits": 0,
  "generated": ""
}
```

### 本地生成方式

```bash
# TS 侧：浏览器内运行 benchmark（webgpu-e2e 测试基座）
cd sdk/typescript && npx vitest run src/llm/__tests__/webgpu-e2e.test.ts
```

### 边缘路由建议

- 敏感数据 → 本地 WebGPU（隐私路由，`PrivacyRouter`）
- 低延迟需求 → 本地小模型（qwen3:0.5b 级）
- 高质量需求 → 云端大模型（`model_router` Balanced 策略）

## 三、Profile 更新节奏

- 每次 WebGPU 相关发布更新一次 profile（报告归档 `docs/benchmarks/edge-*`）
- 目标：首 token < 200ms（本地 0.5B q4）、吞吐 ≥ 30 tok/s
