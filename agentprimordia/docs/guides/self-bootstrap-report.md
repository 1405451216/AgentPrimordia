# AP 自举规模化报告（v4.8-4）

> 目标：AP 用 AP 开发 AP——日常开发用 AP 跑基准集，成功率曲线季度上升。

## 自举工具

```bash
# CLI：多轮自举（ImprovingProvider 包装真实 Provider）
go run ./bench/self-bootstrap --provider openai --model gpt-4o-mini --rounds 5

# 输出：每轮基准集成功率 → 曲线（0.333 → 0.667 → 1.0 示例）
```

实现：`internal/self_bootstrap`（ImprovingProvider + RunBootstrap），
历史实测曲线 0.333 → 0.667 → 1.0（commit 0e3e3ec）。

## 季度报告模板

| 季度 | 基准集规模 | 成功率 | 成本 | 趋势 |
|------|-----------|--------|------|------|
| 2026-Q2（基线） | 60 条真实任务 | 待测 | 待测 | — |
| 2026-Q3 | 60+ | 待测 | 待测 | 待测 |
| 2026-Q4 | 60+ | 待测 | 待测 | 目标 ↑ |

> 填充方式：`go run ./bench/self-bootstrap --rounds 5 --out quarterly.md`，
> 报告随版本发布归档到 `docs/benchmarks/`。

## 规模化用法（自举 → 生产）

1. **日常开发**：CI nightly 以真实 Provider 跑 `bench/llm-bench` + `bench/self-bootstrap`，产出报告（nightly.yml `llm-benchmark` / `dual-bench` job）
2. **回归门**：`perf-regression`（P95 双向偏差 ≤20%）+ llm-bench 分数只升不降（baseline.json）
3. **季度复盘**：成功率曲线上升即自举有效；平台改进（工具/记忆/护栏）以基准集为靶心

## 与 v4.9 联动

自举报告同时产出成本数据（token 用量 + EstimateCost），作为 v4.9-2 token 成本优化的基线。
