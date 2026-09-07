# 工具智能系统示例（Tool Intelligence）

> v7.1 统一工具智能端到端演示

## 概述

本示例展示 AgentPrimordia v7.1 统一工具智能系统的完整使用方式。工具智能系统让 Agent 具备"越用越聪明"的工具使用能力——自动画像、自动调优、自动选择最优工具、自动发现能力缺口并生成新工具。

## 架构

```
                    ┌─────────────────────┐
                    │  ToolIntelligence   │  统一入口
                    └─────────┬───────────┘
          ┌──────────────────┼──────────────────┐
          │                  │                  │
    ┌─────▼─────┐    ┌──────▼──────┐    ┌──────▼──────┐
    │  optimize │    │   create    │    │   reuse     │
    ├───────────┤    ├─────────────┤    ├─────────────┤
    │ Profiler  │    │ GapDetector │    │ ToolCatalog │
    │ Tuner     │    │ Creator     │    │ TaskMatcher │
    │ Selector  │    └─────────────┘    └─────────────┘
    └───────────┘
          │                  │
    ┌─────▼──────────────────▼─────┐
    │     IntelligenceHook         │  桥接 ReAct 循环
    └──────────────────────────────┘
```

## 演示内容

| 步骤 | 组件 | 说明 |
|------|------|------|
| 1 | 全部子组件 | 构造 Profiler / Tuner / Selector / Detector / Creator / Hook / Catalog / Matcher |
| 2 | `reuse.ToolCatalog` + `reuse.TaskMatcher` | 注册工具到目录，按任务描述匹配最佳工具 |
| 3 | `optimize.InMemoryProfiler` | 模拟多次工具调用，计算成功率/延迟/P95 画像 |
| 4 | `optimize.DataDrivenTuner` | 基于画像生成调优建议（低成功率→重试，高延迟→增大超时） |
| 5 | `optimize.HistorySelector` | 记录历史调用结果，从候选中选成功率最高的工具 |
| 6 | `create.TraceGapDetector` + `create.LifecycleCreator` | 分析失败轨迹聚类缺口，自动生成 shell 脚本工具 |
| 7 | `intelligence.IntelligenceHook` | 桥接 ReAct 循环：工具调用后画像记录，轮次结束后缺口检测 |

## 运行

```bash
go run ./ecosystem/examples/tool-intelligence/
```

无需配置 API Key——所有组件使用内存实现，LLM 使用 MockLLM。

## 关键类型

- `intelligence.ToolIntelligence` — 统一入口，组装所有子组件（`intelligence.go`）
- `intelligence.IntelligenceHook` — ReAct 循环桥接 Hook（`hooks.go`）
- `optimize.InMemoryProfiler` — 内存版工具性能画像器（`optimize/profiler.go`）
- `optimize.DataDrivenTuner` — 数据驱动参数调优器（`optimize/tuner.go`）
- `optimize.HistorySelector` — 基于历史成功率的工具选择器（`optimize/selector.go`）
- `create.TraceGapDetector` — 轨迹缺口检测器（`create/detector.go`）
- `create.LifecycleCreator` — 生命周期工具生成器（`create/creator.go`）
- `reuse.ToolCatalog` — 工具目录注册表（`reuse/catalog.go`）
- `reuse.TaskMatcher` — 任务-工具关键词匹配器（`reuse/matcher.go`）
