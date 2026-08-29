# AgentPrimordia Official Performance Benchmarks

**Version**: 5.0.0 (2026-Q4 刷新)
**Report Date**: 2026-08-10
**Status**: PASS (All benchmark suites completed)

---

## Environment

| Item | Value |
|------|-------|
| OS | windows/amd64 |
| CPU | Intel(R) Core(TM) Ultra 5 250K Plus (18 cores) |
| Go | 1.26.5 |
| Memory | 24GB |
| Note | Development machine baseline; production standard: 4C/8GB/linux/amd64 |

---

## v4.0 关键路径 P95 延迟（新增）

> 验收标准：关键路径 P95 达标。延迟分布由 `bench/suite/p95_latency_test.go` 测量，
> 采用批次累积策略（Windows 时钟粒度下单次测时不可靠），P95 = 最慢 5% 批次的平均延迟。

| 关键路径 | P50 (ns/op) | P95 (ns/op) | P99 (ns/op) | 说明 |
|----------|-------------|-------------|-------------|------|
| Agent 单轮 | 3,490 | 10,758 | 15,949 | MockLLM, MaxTurns=1 |
| 工具调用 | 4,116 | 11,028 | 19,324 | MockLLM + 文件系统工具 |
| 记忆检索（1K 条目 FTS） | 29,631 | 46,289 | 50,284 | InMemory SQLite FTS |

> 基准文件：`agentprimordia/bench/results/2026-Q4.json`（v4.0 性能基线）。
> 回归门：`scripts/bench-regression-check.sh`（默认基线 2026-Q4，P95 偏差 >20% 即失败）。

---

## QPS (Queries Per Second)

QPS is calculated from `ns/op` values using the formula: **QPS = 10^9 / ns/op**.

### Core Throughput

| Benchmark | ns/op | QPS | B/op | allocs/op | Notes |
|-----------|-------|-----|------|-----------|-------|
| AgentRun (single) | 4,782 | **209,107** | 3,971 | 24 | MockLLM, single Agent complete run |
| Pool 10-concurrent | 270.1* | **3,702,332** | 732.1* | 3.9* | MockLLM, 10 concurrent Dispatch, per-task |
| ToolCalling (MockLLM) | 5,824 | **171,703** | 5,195 | 34 | End-to-end tool calling with MockLLM |

> \* Pool 10-concurrent: raw value 2,701 ns/op is per-dispatch-batch (10 tasks); per-task = 270.1 ns/op.

### MemoryStore Operations

| Operation | ns/op | QPS | B/op | allocs/op |
|-----------|-------|-----|------|-----------|
| MemoryStore Add | 46.28 | **21,607,606** | — | — |
| MemoryStore Search (1K entries) | 30,019 | **33,312** | — | — |

### Cluster Throughput

| Operation | ns/op | QPS | Notes |
|-----------|-------|-----|-------|
| ConsistentHash GetNode (10 nodes) | 25.65 | **38,986,354** | Single key lookup |
| ConsistentHash GetNode (50 nodes) | 34.31 | **29,145,439** | Single key lookup |
| DistributedState Set | 89.22 | **11,208,249** | Key-value write |
| DistributedState Get | 52.17 | **19,168,103** | Key-value read |

### Learning Pipeline Throughput

| Operation | ns/op | QPS | Notes |
|-----------|-------|-----|-------|
| Distill (single) | 2,642 | **378,501** | Knowledge distillation per interaction |
| Pipeline (full) | 14,777 | **67,673** | Complete distill pipeline |
| BuildSystemPrompt | 8,871 | **112,727** | System prompt assembly from knowledge |
| FeedbackLearner | 1,865 | **536,193** | Feedback recording |
| CapabilityEvolver | 2,146 | **465,983** | Capability evaluation |

### Privacy Router

| Operation | ns/op | QPS | Notes |
|-----------|-------|-----|-------|
| Route (NoPII) | 57.39 | **17,424,638** | No PII detected, direct pass |
| Route (WithPII) | 2,007 | **498,256** | PII detected, redaction applied |
| RegisterCapability | 66.31 | **15,080,681** | Privacy node capability registration |

---

## P99 Latency

Latency data from the latency benchmark suite (`bench/suite/latency_test.go`).

> Note: Values below are mean latencies from MockLLM benchmarks. P99 estimates are derived from
> multi-run distributions. With MockLLM, actual LLM network latency is excluded; production
> latency will be dominated by LLM API round-trip time.

| Metric | Mean Latency | P99 Estimate | Target | Status |
|--------|-------------|--------------|--------|--------|
| Agent single-turn latency | 5.39 µs | ~7.5 µs | < 500 ms | PASS |
| FirstToken streaming latency | 18.31 µs | ~25 µs | < 100 ms | PASS |
| Tool calling latency (MockLLM) | 5.82 µs | ~8 µs | < 5 ms | PASS |
| Memory Search (1K entries) | 29.48 µs | ~42 µs | < 1 ms | PASS |
| Vector Search (10K vectors) | 1.84 ms | ~2.4 ms | < 10 ms | PASS |

---

## Memory Usage

Memory consumption data from capacity and core benchmark suites.

| Metric | Value | Notes |
|--------|-------|-------|
| Single Agent (per-run allocation) | 3,971 B/op | 24 allocs/op, MockLLM |
| Pool 10-concurrent (per-dispatch) | 7,321 B/op | 39 allocs/op, 10 tasks per dispatch |
| ToolCalling Agent (per-run) | 5,195 B/op | 34 allocs/op, with tool registry |
| Vector Store (10K vectors, 128-dim) | 881,976 B/op | ~862 KB for 10K vectors |
| MemoryStore (1K entries) | 1,345 B/op per search | 31 allocs/op per search |
| WASM module (reference) | ~30-50 µs execution | See `wasm/bench_test.go`; far below 5 ms target |

### 100-Concurrent Agent Capacity

| Metric | Value | Target | Status |
|--------|-------|--------|--------|
| 100 concurrent agents P99 | < 500 ms | < 500 ms | PASS |
| Total dispatch time | Measured in `TestCapacity_SingleNode_100ConcurrentAgents` | — | PASS |

---

## Vector Search Performance

| Dataset Size | Dimensions | Search Latency | Throughput | Memory |
|-------------|-----------|---------------|------------|--------|
| 10,000 vectors | 128 | 1.84 ms (mean) | 543 QPS | 862 KB |
| 1,000 entries (text) | — | 29.48 µs (mean) | 33,312 QPS | 1.3 KB/op |

Vector search uses brute-force cosine similarity. For production workloads with millions of vectors,
integrate `pgvector` (provided in `pgvector/` module) for indexed ANN search.

---

## Cluster Performance

### ConsistentHash Throughput by Node Count

| Nodes | ns/op | QPS | B/op | allocs/op |
|-------|-------|-----|------|-----------|
| 3 | 19.66 | 50,869,797 | — | — |
| 5 | 25.30 | 39,525,692 | — | — |
| 10 | 25.65 | 38,986,354 | — | — |
| 20 | 33.58 | 29,779,630 | — | — |
| 50 | 34.31 | 29,145,439 | — | — |

### DistributedState Operations

| Operation | ns/op | QPS |
|-----------|-------|-----|
| Set | 89.22 | 11,208,249 |
| Get | 52.17 | 19,168,103 |
| SetGet Mixed | ~86 | ~11,627,907 |
| TTL Set | ~90 | ~11,111,111 |

---

## Learning Pipeline Performance

### Knowledge Distillation

| Operation | ns/op | QPS | Description |
|-----------|-------|-----|-------------|
| Distill | 2,642 | 378,501 | Single interaction distillation |
| Pipeline ProcessInteraction | 14,777 | 67,673 | Full pipeline with validation |
| BuildSystemPrompt | 8,871 | 112,727 | Prompt assembly from distilled knowledge |

### Adaptive Learning

| Operation | ns/op | QPS | Description |
|-----------|-------|-----|-------------|
| FeedbackLearner RecordFeedback | 1,865 | 536,193 | Record and process user feedback |
| CapabilityEvolver Evaluate | 2,146 | 465,983 | Evaluate capability score update |

---

## How to Reproduce

### Prerequisites

- Go 1.26.5 or later
- Clone the AgentPrimordia repository
- No external LLM API key required (benchmarks use MockLLM)

### Run All Benchmarks

```bash
cd agentprimordia
go test -bench=. -benchmem -benchtime=100ms -run=^$ -count=1 ./bench/suite/
```

### Run Specific Suites

```bash
# Core throughput (AgentRun, MemoryStore, ToolCalling)
go test -bench=BenchmarkAgentRun -benchmem -benchtime=100ms -count=1 ./bench/suite/
go test -bench=BenchmarkMemoryStore -benchmem -benchtime=100ms -count=1 ./bench/suite/
go test -bench=BenchmarkToolCalling -benchmem -benchtime=100ms -count=1 ./bench/suite/

# Latency (Agent, FirstToken, Concurrent, Vector, Memory)
go test -bench=BenchmarkLatency -benchmem -benchtime=100ms -count=1 ./bench/suite/
go test -bench=BenchmarkFirstTokenLatency -benchmem -benchtime=100ms -count=1 ./bench/suite/
go test -bench=BenchmarkVectorSearch -benchmem -benchtime=100ms -count=1 ./bench/suite/

# Cluster
go test -bench=BenchmarkConsistentHash -benchmem -benchtime=100ms -count=1 ./bench/suite/
go test -bench=BenchmarkDistributedState -benchmem -benchtime=100ms -count=1 ./bench/suite/

# Learning
go test -bench=BenchmarkKnowledgeDistiller -benchmem -benchtime=100ms -count=1 ./bench/suite/
go test -bench=BenchmarkDistillPipeline -benchmem -benchtime=100ms -count=1 ./bench/suite/
go test -bench=BenchmarkFeedbackLearner -benchmem -benchtime=100ms -count=1 ./bench/suite/
go test -bench=BenchmarkCapabilityEvolver -benchmem -benchtime=100ms -count=1 ./bench/suite/

# Privacy
go test -bench=BenchmarkPrivacyRouter -benchmem -benchtime=100ms -count=1 ./bench/suite/

# QPS (direct measurement)
go test -bench=BenchmarkOfficial_QPS -benchmem -benchtime=100ms -count=1 ./bench/suite/
```

### Generate CPU Profile

```bash
go test -bench=BenchmarkAgentRun -cpuprofile=cpu.prof -benchmem ./bench/suite/
go tool pprof cpu.prof
```

### Run Capacity Tests

```bash
go test -v -run=TestCapacity -count=1 ./bench/suite/
```

### Run with Multiple Iterations (stable results)

```bash
go test -bench=. -benchmem -count=5 -run=^$ ./bench/suite/ | tee bench-results.txt
```

---

## Trend Comparison Framework

Use this template for quarterly trend comparisons (e.g., Q3 vs Q2):

```markdown
## Q3 2026 vs Q2 2026 Trend

| Metric | Q2 Baseline | Q3 Current | Change | Status |
|--------|-----------|-----------|--------|--------|
| AgentRun QPS | 209,107 | TBD | TBD% | TBD |
| Pool 10-concurrent QPS | 3,702,332 | TBD | TBD% | TBD |
| MemoryStore Add QPS | 21,607,606 | TBD | TBD% | TBD |
| MemoryStore Search QPS | 33,312 | TBD | TBD% | TBD |
| Vector Search 10K (ms) | 1.84 | TBD | TBD% | TBD |
| Agent single-turn (µs) | 5.39 | TBD | TBD% | TBD |
| FirstToken streaming (µs) | 18.31 | TBD | TBD% | TBD |
| Tool calling (µs) | 5.82 | TBD | TBD% | TBD |
| ConsistentHash 10-node QPS | 38,986,354 | TBD | TBD% | TBD |
| DistributedState Set QPS | 11,208,249 | TBD | TBD% | TBD |
| Distill QPS | 378,501 | TBD | TBD% | TBD |
| Pipeline QPS | 67,673 | TBD | TBD% | TBD |
```

### Regression Thresholds

| Threshold | Action |
|-----------|--------|
| < 5% change | PASS — within noise margin |
| 5-15% change | WARNING — investigate, may be environmental |
| 15-30% change | FAIL — requires root cause analysis |
| > 30% change | CRITICAL — block release until resolved |

---

## Skipped Benchmarks

| Benchmark | Reason |
|-----------|--------|
| Tool Calling Accuracy (50 scenarios) | Requires real LLM API Key; MockLLM cannot measure real tool call accuracy |
| RAG Recall@5 | Requires real LLM API Key and vector database integration |

---

## References

- Benchmark suite source: [`bench/suite/`](../../bench/suite/)
- Raw results: [`bench/results/2026-Q2.json`](../../bench/results/2026-Q2.json)
- Regression report: [`bench/results/2026-Q2-regression-report.md`](../../bench/results/2026-Q2-regression-report.md)
- Benchmark runner guide: [`bench/README.md`](../../bench/README.md)
- Results format: [`bench/results/README.md`](../../bench/results/README.md)

---

*Generated from 2026-Q2 baseline data. All measurements use MockLLM; production performance
with real LLM providers will differ significantly due to network latency.*

---

## v5.0 Studio 后端压测（新增）

> 来源：`bench/suite/studio_load_test.go` + `bench/soak/studio_soak_test.go`，报告见 `bench/results/studio-load-report.md`。

| 视角 | 结果 |
|------|------|
| 读路径并发（100 并发 × 9 端点 × 200 轮） | 20000 请求 0 错误（~31.7k req/s） |
| 写路径并发（POST chaos 5000 + deploy 2500） | 0 错误，写后读数量一致 |
| 端点延迟 P50/P95/P99 | 全部亚毫秒（P95 ≤ 104µs） |
| 轮询节奏模拟（2s/5s/10s × 30s） | 0 错误 |
| 极限（1000 目标全量读取 × 50 轮） | 0 错误，数据完整，最大 6.8ms |
| **30 分钟稳态（88182 请求，50 rps 混合读写）** | **0 错误、无退化、平均延迟 208µs** |

> 压测检出并修复：demo 存储无界膨胀（30 分钟延迟 +246% → 有界保留 1000 后 -84%，退化归零）。
