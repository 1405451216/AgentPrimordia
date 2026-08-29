# AgentPrimordia Benchmark

性能基准测试套件，用于量化评估和跨框架对比。

## 运行基准

```bash
# 全部基准
cd bench && go test -bench=. -benchmem ./suite/

# 单项基准
go test -bench=BenchmarkAgentRun -benchmem ./suite/
go test -bench=BenchmarkConcurrent -benchmem ./suite/
go test -bench=BenchmarkVectorSearch -benchmem ./suite/

# 生成 CPU Profile
go test -bench=BenchmarkAgentRun -cpuprofile=cpu.prof ./suite/
go tool pprof cpu.prof
```

## 基准指标

| 基准 | 指标 | 说明 |
|------|------|------|
| `BenchmarkToolCalling` | ops/sec | 工具调用准确率和吞吐量 |
| `BenchmarkAgentRun` | ops/sec | 单 Agent 运行吞吐量 |
| `BenchmarkConcurrent` | ops/sec | 10 并发 Agent Pool 吞吐量 |
| `BenchmarkFirstTokenLatency` | ns/op | 首 Token 延迟 |
| `BenchmarkMemoryStore` | ns/op | 记忆写入和搜索延迟 |
| `BenchmarkVectorSearch` | ns/op | 10K 向量搜索延迟 |

## 对比基线

本仓库不维护其他框架（如 LangChain）的等价实现代码；横向对比以基线数据的形式落盘于 `results/` 目录（JSON 基线 + 季度记录），后续评测可与历史基线直接比对。

## 结果发布

每季度在 `results/` 目录发布基准结果，格式为 JSON。

在 GitHub Pages 上托管排行榜：`https://agentprimordia.dev/bench`
