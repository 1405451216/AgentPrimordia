# PGO（Profile-Guided Optimization）指南

> Go 1.21+ 支持基于 CPU profile 的编译器优化，可提升 2-7% 运行时性能。

## 原理

PGO 的工作流程：

1. **采集**：运行代表性工作负载，生成 CPU profile（pprof 格式）
2. **标注**：编译器读取 profile，识别热点函数和调用路径
3. **优化**：对热点代码进行更激进的内联、devirtualization 和寄存器分配

与传统 AOT 优化不同，PGO 基于真实运行数据而非静态分析，能更精准地优化实际执行路径。

## 快速开始

### 1. 生成 Profile

```bash
make pgo-profile
```

这会运行 `bench/suite/` 下的基准测试并生成 `.pgo/cpu.prof`。

> **重要**：PGO 的效果取决于 profile 的代表性。如果生产工作负载与基准测试差异较大，
> 建议在生产环境使用 pprof 端点采集真实流量 profile（见下方「生产采集」章节）。

### 2. 构建 PGO 优化二进制

```bash
make pgo-build
```

生成的 `ap` 二进制已包含 PGO 优化。

### 3. 验证效果

```bash
# 标准构建
make build
go test -bench=. -benchtime=10x ./bench/suite/... > before.txt

# PGO 构建
make pgo-build
go test -bench=. -benchtime=10x ./bench/suite/... > after.txt

# 对比
benchstat before.txt after.txt
```

### 4. 清理

```bash
make pgo-clean
```

## 生产环境 Profile 采集

对于真实工作负载，建议从生产环境采集 profile：

### 方式一：pprof 端点（推荐）

项目已集成 pprof 端点（见 `internal/health/pprof.go`）：

```go
mux := http.NewServeMux()
ap.RegisterPProfSecure(mux)
go http.ListenAndServe("localhost:6060", mux)
```

采集 30 秒 CPU profile：

```bash
curl -o cpu.prof http://localhost:6060/debug/pprof/profile?seconds=30
```

### 方式二：go test -cpuprofile

如果工作负载可通过测试复现：

```bash
go test -run=^$ -bench=. -benchtime=100x -cpuprofile=cpu.prof ./bench/suite/...
```

## CI/CD 集成

在 CI 中使用 PGO 的推荐做法：

```yaml
# GitHub Actions 示例
- name: Generate PGO Profile
  run: make pgo-profile

- name: Build with PGO
  run: make pgo-build

- name: Verify PGO
  run: |
    go tool pprof -top .pgo/cpu.prof | head -20
```

### 注意事项

1. **Profile 时效性**：代码大幅变更后应重新采集 profile
2. **版本控制**：`default.pgo` 应提交到 Git，确保构建可复现
3. **Go 版本**：需 Go 1.21+，建议使用最新稳定版以获得最佳优化效果
4. **CGO 兼容**：本项目 `CGO_ENABLED=0`，PGO 完全兼容

## 常见问题

### Q: PGO 为什么没有效果？

可能原因：
- 工作负载以 I/O 为主（CPU 热点不集中）
- Go 版本过低（1.21 之前的版本不支持 PGO）
- Profile 不具代表性（基准测试未覆盖热点路径）
- 代码变更后未重新采集 profile

### Q: PGO 和 -ldflags -s -w 冲突吗？

不冲突。两者作用于不同阶段：
- PGO 影响编译期的代码优化
- `-s -w` 在链接期剥离调试信息

可同时使用：`go build -pgo=auto -ldflags="-s -w" -o ap ./cmd/ap/`

### Q: 如何验证 PGO 是否生效？

```bash
# 查看编译器是否使用了 PGO
go build -pgo=default.pgo -gcflags="-m" ./cmd/ap/ 2>&1 | grep "pgo"
```

输出中应包含 `pgo` 相关的优化决策信息。

## 参考

- [Go 官方 PGO 文档](https://go.dev/doc/pgo)
- [PGO 设计提案](https://go.googlesource.com/proposal/+/master/design/55022-pgo.md)
- [Go 1.21 Release Notes](https://go.dev/doc/go1.21)

## perf-v12 实测（2026-07-03）

### 流程首次端到端跑通

```bash
# 1. 采集代表性 profile
go test -count=1 -run='^$' -bench='.' -benchmem -benchtime=3s -cpuprofile=default.pgo \
  ./bench/suite/latency_test.go ./bench/suite/tool_calling_test.go

# 2. 用 PGO 跑 bench 对比
go test -count=1 -run='^$' -bench='.' -benchmem -benchtime=3s -pgo=default.pgo \
  ./bench/suite/latency_test.go ./bench/suite/tool_calling_test.go
```

### 实测收益（Windows 11, Go 1.26.3, AMD Ryzen Ultra 5 250K）

| Bench | 无 PGO | 有 PGO | 改善 |
|---|---:|---:|---:|
| BenchmarkMemoryStore/Add | 46.99 ns/op | 38.39 ns/op | **-18.3%** ⬇️ |
| BenchmarkMemoryStore/Search | 27115 ns/op | 26452 ns/op | -2.4% |
| ThroughputAgent | 4530 ns/op | 4447 ns/op | -1.8% |
| Memory alloc (Throughput) | 2898 B/op | 2946 B/op | +1.7% ⬆️ |

**说明**：
- `MemoryStore/Add` 18.3% 提升远超 README 承诺的 2-7% — 推测 `go test -pgo=...` 启用了内联和寄存器分配优化
- 其他 bench 提升较小，因为这些 bench 的 hot path 在 runtime 内部（GC/调度），PGO 无法优化

### 注意事项（perf-v12 实测补充）

1. **profile 与 CPU 架构相关**：Intel 上采的 profile 在 AMD 上 build 可能无收益或反效果。建议 CI 任务自己跑 `make pgo-profile` 重新生成
2. **.gitignore 已排除 `default.pgo`**：每个 release 流程需重新采集
3. **bench/suite/ 覆盖窄**：当前只有 `latency_test.go` + `tool_calling_test.go`，未来补 RAG/Vector/Pool/DAG bench 后 PGO 收益会更全面
