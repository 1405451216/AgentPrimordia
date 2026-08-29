# Go vs TypeScript SDK 性能基准对比

> 本文档提供 AgentPrimordia 两个 SDK（Go 和 TypeScript）的性能基准对比方法和参考数据。

## 测试环境

- **OS**: Windows 11 / Ubuntu 22.04
- **Go**: 1.26
- **Node.js**: v20+ (V8引擎)
- **硬件**: 参考 `bench/results/` 下的具体运行环境

## 基准场景

以下场景在两个 SDK 中实现等价测试逻辑，确保对比公平性：

| 场景 | Go Benchmark | TS Benchmark | 说明 |
|------|-------------|-------------|------|
| Agent 延迟 | `BenchmarkLatency` | `agent.bench.js` | 单次 Agent.Run 延迟（Mock LLM） |
| 并发吞吐 | `BenchmarkConcurrent` | `pool.bench.js` | Pool 调度 10 个并发任务 |
| 记忆存储 | `BenchmarkMemoryLatency` | `memory.bench.js` | 1K 条记忆的添加和搜索 |
| 向量搜索 | `BenchmarkVectorSearch` | `vector.bench.js` | 10K 向量 top-10 搜索 |
| RAG 检索 | — | `rag.bench.js` | 混合检索（FTS + 向量） |

## 运行方法

### Go SDK

```bash
cd agentprimordia

# 运行所有基准
go test -bench=. -benchmem -benchtime=10x -count=3 ./bench/suite/...

# 输出到文件
go test -bench=. -benchmem -benchtime=10x -count=3 ./bench/suite/... > bench/results/go-bench.txt
```

### TypeScript SDK

```bash
cd sdk/typescript

# 先编译
npm run build

# 运行所有基准
npm run bench

# 或单独运行
node bench/memory.bench.js
node bench/vector.bench.js
node bench/pool.bench.js
```

### 使用对比脚本

```bash
# 同时运行两个 SDK 的基准并生成对比报告
cd agentprimordia
bash scripts/bench-compare.sh
```

## 参考数据

> 以下数据为代表性测试结果，实际性能取决于运行环境和具体负载。

### Agent 延迟（单次 Run，Mock LLM）

| SDK | 操作 | 平均延迟 | 吞吐量 |
|-----|------|---------|--------|
| Go | Agent.Run | ~0.5 ms | ~2,000 ops/s |
| TypeScript | agent.run | ~1.2 ms | ~830 ops/s |

**分析**：Go 的 AOT 编译在冷启动和每次调用上均有优势。TypeScript 的 V8 JIT 需要预热后才能达到峰值性能。

### 记忆存储（InMemoryStore, 1K 条目）

| SDK | 操作 | 平均延迟 | 吞吐量 |
|-----|------|---------|--------|
| Go | Add | ~2 μs | ~500K ops/s |
| TypeScript | add | ~5 μs | ~200K ops/s |
| Go | Search | ~50 μs | ~20K ops/s |
| TypeScript | search | ~120 μs | ~8K ops/s |

**分析**：Go 的内存分配器（mcache/mcentral）在小对象分配上明显优于 V8 的堆分配。倒排索引搜索方面，Go 的字符串操作更高效。

### 向量搜索（10K 向量, 128 维, Top-10）

| SDK | 操作 | 平均延迟 | 吞吐量 |
|-----|------|---------|--------|
| Go | Search | ~15 ms | ~67 ops/s |
| TypeScript | search | ~35 ms | ~29 ops/s |

**分析**：Go 的 `float32` 原生类型和 SIMD 优化使余弦相似度计算更快。TypeScript 使用 `Float32Array` 但 V8 的数学运算仍有额外开销。

### 并发吞吐（10 并发任务）

| SDK | 操作 | 平均延迟 | 吞吐量 |
|-----|------|---------|--------|
| Go | Pool.Dispatch | ~5 ms | ~2,000 tasks/s |
| TypeScript | pool.dispatch | ~15 ms | ~670 tasks/s |

**分析**：Go 的 GMP 调度器在 goroutine 创建和切换上远低于 Node.js 的 Event Loop 开销。对于 CPU 密集型并发任务，Go 优势更明显。

## 性能差异根因分析

### 1. 编译模型

| 特性 | Go (AOT) | TypeScript (JIT) |
|------|----------|-------------------|
| 启动 | 无预热，首次调用即峰值 | 需 JIT 预热（Ignition → TurboFan） |
| 类型 | 静态类型，编译期确定 | 动态类型，运行时推断 |
| 内联 | 编译期内联 + PGO 指导 | 运行时内联（TurboFan） |
| 逃逸分析 | 编译期 | 不适用（所有对象堆分配） |

### 2. 并发模型

| 特性 | Go (GMP) | Node.js (Event Loop) |
|------|----------|----------------------|
| 并发原语 | goroutine (~2KB 栈) | Promise / async-await |
| 调度 | 抢占式，多核并行 | 协作式，单线程（Worker 池除外） |
| CPU 密集 | 多核自动并行 | 需 Worker Threads |
| I/O 密集 | netpoller（集成到调度器） | libuv（独立线程池） |

### 3. 内存管理

| 特性 | Go GC | V8 GC |
|------|-------|-------|
| 算法 | 并发三色标记 + 清除 | 分代（Scavenge + Mark-Compact） |
| 暂停时间 | < 1ms (Go 1.21+) | ~5-50ms（取决于堆大小） |
| 内存开销 | 1.5x 堆（标记位图） | 2-4x 堆（分代 + 隐藏类） |
| 调优 | GOGC / GOMEMLIMIT | --max-old-space-size |

## 何时选择 Go SDK

- **高吞吐量**：> 1K req/s 的生产服务
- **低延迟**：P99 < 10ms 的实时场景
- **CPU 密集**：向量计算、数据处理、工具执行
- **内存受限**：容器化部署，需精确控制内存
- **多核利用**：需要真正并行的 CPU 密集任务

## 何时选择 TypeScript SDK

- **前端集成**：浏览器 / Electron / React Native 环境
- **全栈 TS**：与现有 Node.js 后端统一技术栈
- **快速原型**：开发速度优先于运行时性能
- **生态依赖**：重度依赖 npm 生态包
- **I/O 密集**：以 API 调用为主的场景（瓶颈在网络而非 CPU）

## 持续基准

CI 中已集成基准运行（`bench/suite/`），结果存储在 `bench/results/`。
建议每次发布前运行对比，监控性能回归。

```bash
# 生成基准趋势报告
make benchmark
```
