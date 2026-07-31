# Benchmark Results

本目录存放 AgentPrimordia 各季度/版本的性能基准测试结果。

## 文件命名规范

| 文件 | 说明 |
|------|------|
| `<year>-Q<quarter>.json` | 季度基准结果（机器可读） |
| `<year>-Q<quarter>-regression-report.md` | 回归分析报告（人类可读） |
| `*.prof` | Go pprof 性能剖析文件 |

## JSON 结果格式

```jsonc
{
  "date": "2026-Q2",           // 测试周期
  "version": "3.1.0",         // 框架版本
  "environment": {            // 运行环境
    "go": "1.26.5",
    "os": "windows/amd64",
    "cpu": "Intel(R) Core(TM) Ultra 5 250K Plus",
    "memory": "24GB",
    "note": "..."
  },
  "status": "completed",      // completed | partial | skipped
  "run_instructions": "...",  // 复现命令
  "results": {                // 核心指标
    "<metric_name>": {
      "description": "指标描述",
      "unit": "ns/op | ms | %",
      "value": 4782,
      "note": "补充说明"
    }
  },
  "v31_suites": {             // V3.1 套件结果
    "<suite_name>": {
      "description": "套件描述",
      "file": "bench/suite/xxx_test.go",
      "value": "PASS",
      "details": "关键数据摘要"
    }
  }
}
```

## 核心指标说明

| 指标 | 含义 | 计算方式 |
|------|------|---------|
| QPS | 每秒查询数 | `10^9 / ns/op` |
| P99 延迟 | 99 分位延迟 | 排序后第 99% 样本值 |
| B/op | 每次操作分配字节数 | `go test -benchmem` 输出 |
| allocs/op | 每次操作堆分配次数 | `go test -benchmem` 输出 |

## 运行方法

```bash
cd agentprimordia

# 运行全部基准套件
go test -bench=Benchmark -benchmem -benchtime=100ms -run=^$ -count=1 ./bench/suite

# 运行特定套件
go test -bench=BenchmarkAgentRun -benchmem -run=^$ ./bench/suite/

# 生成 CPU profile
go test -bench=. -cpuprofile=cpu.prof -run=^$ ./bench/suite/
```

## 趋势对比

每次发布新版本时，将新结果 JSON 与上一版本进行对比：

1. 相同 `metric_name` 的 `value` 变化超过 ±10% 时标记为回归/提升
2. 回归分析报告记录在 `<year>-Q<quarter>-regression-report.md`
3. 官方汇总数据发布在 `docs/benchmarks/official-benchmarks.md`
