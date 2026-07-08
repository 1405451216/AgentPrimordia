# 阶段四：可观测性与运维成熟度实施计划（2-3 周）

> **状态：已完成 ✅**（10/10 Task 全部完成，2026-07-06）
> **创建日期：2026-07-05**

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## 目标

构建端到端可观测性体系（分布式追踪 + Metrics + 结构化日志）、完善 Grafana Dashboard 和告警规则、升级 K8s Operator 实现自定义指标自动扩缩容，使 AgentPrimordia 达到生产级运维成熟度。

## 当前状态盘点

| 组件 | 状态 | 说明 |
|------|------|------|
| OTel Bridge (`internal/otel/`) | ✅ 完成 | 内建实现，生成真实 trace/span ID，支持 OTLP 导出 |
| Metrics (`internal/metrics/`) | ✅ 完成 | Prometheus 格式，3 维标签（provider/model/tool_name） |
| Metrics HTTP Handler | ✅ 完成 | `/metrics` 端点 |
| Health Check (`internal/health/`) | ✅ 完成 | `/healthz`、`/readyz`、`/livez` |
| pprof 端点 | ✅ 完成 | `ap.RegisterPProf(mux)` |
| Grafana Dashboard | ✅ 部分 | 3 个 dashboard JSON（agent/llm/cost），缺告警规则 |
| OTLP Exporter | ✅ 完成 | `internal/otel/otlp_exporter.go` |
| K8s Operator | ✅ 部分 | CRD + HPA + Pod 指标采集，缺自定义 Metrics Adapter |
| PDB | ⬜ 未开始 | 无 Pod Disruption Budget |
| SLO 告警 | ⬜ 未开始 | 无 Prometheus alerting rules |

---

## Phase 4A：分布式追踪增强（第 1-4 天）

### Task 1: 跨 Agent 追踪链路传播

**问题**：A2A 调用时 trace context 未自动传播，无法跨 Agent 串联完整调用链。

**Files:**
- Modify: `internal/agent/a2a/grpc_client.go`（注入 trace context 到 gRPC metadata）
- Modify: `internal/agent/a2a/grpc_server.go`（从 gRPC metadata 提取 trace context）
- Create: `internal/agent/a2a/trace_propagation_test.go`

- [x] **Step 1: 客户端注入 W3C Trace Context**

```go
// internal/agent/a2a/grpc_client.go
func (c *A2AGRPCClient) SendTask(ctx context.Context, req *a2av1.SendTaskRequest) (*a2av1.SendTaskResponse, error) {
    // 注入 trace context 到 gRPC metadata
    ctx = injectTraceContext(ctx)
    return c.client.SendTask(ctx, req)
}

func injectTraceContext(ctx context.Context) context.Context {
    // 从 ctx 中获取 span，提取 traceparent header
    // 注入到 gRPC metadata
    md, ok := metadata.FromOutgoingContext(ctx)
    if !ok {
        md = metadata.New(nil)
    }
    // 生成 W3C traceparent: 00-<trace-id>-<span-id>-<flags>
    traceparent := buildTraceparent(ctx)
    md.Append("traceparent", traceparent)
    return metadata.NewOutgoingContext(ctx, md)
}
```

- [x] **Step 2: 服务端提取 Trace Context**

```go
// internal/agent/a2a/grpc_server.go
func (s *A2AGRPCServer) SendTask(ctx context.Context, req *a2av1.SendTaskRequest) (*a2av1.SendTaskResponse, error) {
    // 从 gRPC metadata 提取 trace context
    ctx = extractTraceContext(ctx)
    
    // 创建子 span（继承父 trace）
    span := otel.StartSpanFromContext(ctx, "a2a.SendTask")
    defer span.End()
    
    return s.handler.SendTask(ctx, req)
}
```

- [x] **Step 3: 编写测试**

```go
func TestTracePropagation_A2ACall(t *testing.T) {
    // Agent A → A2A → Agent B
    // 验证 Agent B 的 span 继承 Agent A 的 trace ID
}
```

- [x] **Step 4: 验证**

```bash
go test -race -count=1 ./internal/agent/a2a/ -run TestTrace
```

---

### Task 2: 编排层自动追踪

**问题**：Pipeline/DAG/Handoff 节点未自动创建子 span，编排层调用链不可见。

**Files:**
- Modify: `internal/orchestration/orchestrator.go`
- Modify: `internal/orchestration/pipeline.go`
- Modify: `internal/orchestration/handoff.go`
- Modify: `internal/agent/dag.go`（或拆分后的 `internal/agent/dag/`）

- [x] **Step 1: 在编排引擎中注入 tracer**

```go
type Orchestrator struct {
    // ... 现有字段
    tracer Tracer // 可选，nil 表示不追踪
}

func (o *Orchestrator) executeSequential(ctx context.Context, steps []Step) error {
    for i, step := range steps {
        span := o.tracer.StartSpan(fmt.Sprintf("orchestration.step.%s", step.ID))
        span.SetAttribute("step.index", i)
        span.SetAttribute("step.name", step.Name)
        
        err := step.Handler(ctx, input)
        
        if err != nil {
            span.SetAttribute("error", err.Error())
            span.End()
            return err
        }
        span.End()
    }
    return nil
}
```

- [x] **Step 2: DAG 每个节点自动追踪**

```go
func (d *DAGWorkflow) executeNode(ctx context.Context, nodeID string) error {
    span := d.tracer.StartSpan(fmt.Sprintf("dag.node.%s", nodeID))
    span.SetAttribute("dag.name", d.name)
    span.SetAttribute("node.id", nodeID)
    defer span.End()
    
    // ... 节点执行逻辑
}
```

- [x] **Step 3: LLM 调用追踪增强**

```go
// 在 LLM 调用处增加 span attribute
span.SetAttribute("llm.provider", provider)
span.SetAttribute("llm.model", model)
span.SetAttribute("llm.prompt_tokens", resp.Usage.PromptTokens)
span.SetAttribute("llm.completion_tokens", resp.Usage.CompletionTokens)
span.SetAttribute("llm.cost_usd", cost)
```

- [x] **Step 4: 验证**

```bash
go test -race -count=1 ./internal/orchestration/ ./internal/agent/
```

---

## Phase 4B：Grafana Dashboard 完善 + SLO 告警（第 5-8 天）

### Task 3: 补充 Pool/Memory/Orchestration Dashboard

**问题**：当前仅有 3 个 dashboard（agent/llm/cost），缺少 Pool 调度、Memory 存储、Orchestration 编排的可视化。

**Files:**
- Create: `deploy/grafana/dashboard-pool.json`
- Create: `deploy/grafana/dashboard-memory.json`
- Create: `deploy/grafana/dashboard-orchestration.json`

- [x] **Step 1: Pool Dashboard**

面板包含：
- Active Workers 时序图
- Queue Length 时序图
- Task Dispatch Rate
- Task Duration P50/P95/P99
- Task Error Rate
- Worker Scaling Events

- [x] **Step 2: Memory Dashboard**

面板包含：
- Memory Size (bytes) 时序图
- Episode Count 时序图
- FTS Search Latency
- HNSW Search Latency
- RAG Retrieval Count
- Memory Compression Events

- [x] **Step 3: Orchestration Dashboard**

面板包含：
- Workflow Execution Rate
- Step Duration by Type
- DAG Node Execution Time
- Handoff Protocol Latency
- Collaboration Round Duration
- Pipeline Throughput

---

### Task 4: Prometheus Alerting Rules

**Files:**
- Create: `deploy/grafana/alerting_rules.yml`

- [x] **Step 1: 定义 SLO 告警规则**

```yaml
# deploy/grafana/alerting_rules.yml
groups:
  - name: agentprimordia-slo
    rules:
      # LLM 调用 P99 延迟 > 5s
      - alert: LLMHighLatency
        expr: histogram_quantile(0.99, rate(ap_llm_latency_ms_bucket[5m])) > 5000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "LLM P99 latency > 5s"
          description: "Provider {{ $labels.provider }} Model {{ $labels.model }} P99 = {{ $value }}ms"

      # LLM 错误率 > 5%
      - alert: LLMHighErrorRate
        expr: rate(ap_llm_total_errors[5m]) / rate(ap_llm_total_calls[5m]) > 0.05
        for: 2m
        labels:
          severity: critical

      # 缓存命中率 < 80%
      - alert: LowCacheHitRate
        expr: rate(ap_cache_hits[5m]) / rate(ap_cache_total[5m]) < 0.8
        for: 10m
        labels:
          severity: warning

      # Pool 队列积压
      - alert: PoolQueueBacklog
        expr: ap_pool_queue_length > 100
        for: 1m
        labels:
          severity: warning

      # Agent panic
      - alert: AgentPanic
        expr: increase(ap_agent_panics[1m]) > 0
        labels:
          severity: critical

      # 成本超预算
      - alert: BudgetExceeded
        expr: ap_total_cost_usd > 100
        labels:
          severity: critical

      # Goroutine 泄漏
      - alert: GoroutineLeak
        expr: go_goroutines > 10000
        for: 5m
        labels:
          severity: warning
```

- [x] **Step 2: 更新 Grafana datasource.yml**

确保 Prometheus datasource 配置指向正确的 alertmanager。

- [x] **Step 3: 创建 Alerting Contact Points 文档**

```yaml
# deploy/grafana/contact_points.yml
# Slack / Email / PagerDuty 配置模板
```

---

### Task 5: 成本监控面板增强

**Files:**
- Modify: `deploy/grafana/dashboard-cost.json`

- [x] **Step 1: 添加实时成本面板**

面板包含：
- 每日 LLM Token 消耗（按 provider/model 分组）
- 每日成本（USD）时序图
- 预算消耗进度条
- 成本趋势预测线（7 天移动平均）
- Top 10 最贵 Agent 排行

---

## Phase 4C：K8s Operator 升级（第 9-12 天）

### Task 6: 自定义 Metrics Adapter

**问题**：HPA 当前基于 CPU/内存和简单的 Pod 指标，无法基于 `concurrent_tasks_per_pod` 等自定义指标自动扩缩容。

**Files:**
- Create: `operator/controller/metrics_adapter.go`
- Modify: `operator/controller/agent_controller.go`
- Modify: `operator/api/v1/types.go`（增加 HPA 自定义指标配置）

- [x] **Step 1: 实现 Pod 指标采集器**

```go
// operator/controller/metrics_adapter.go
type PodMetricsCollector struct {
    client client.Client
}

// Collect 从每个 Pod 的 /metrics 端点采集指标
func (c *PodMetricsCollector) Collect(ctx context.Context, deploy *agentv1.AgentDeployment) (CustomMetrics, error) {
    pods := c.getPodsForDeployment(ctx, deploy)
    
    var total float64
    for _, pod := range pods {
        // 请求 Pod 的 /metrics 端点
        metricsURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/metrics", pod.Name, pod.Namespace)
        resp, err := http.Get(metricsURL)
        // 解析 ap_concurrent_tasks 指标
        // 累加到 total
    }
    
    return CustomMetrics{
        ConcurrentTasksPerPod: total / float64(len(pods)),
        TotalConcurrentTasks:  total,
    }, nil
}
```

- [x] **Step 2: HPA v2 配置自定义指标**

```go
func (r *AgentDeploymentReconciler) ensureHPA(ctx context.Context, deploy *agentv1.AgentDeployment) error {
    hpa := &autoscalingv2.HorizontalPodAutoscaler{
        Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
            Metrics: []autoscalingv2.MetricSpec{
                {
                    Type: autoscalingv2.PodsMetricSourceType,
                    Pods: &autoscalingv2.PodsMetricSource{
                        Metric: autoscalingv2.MetricIdentifier{
                            Name: "concurrent_tasks_per_pod",
                        },
                        Target: autoscalingv2.MetricTarget{
                            Type:         autoscalingv2.AverageValueMetricType,
                            AverageValue: resource.NewQuantity(5, resource.DecimalSI), // 每 Pod 最多 5 个并发任务
                        },
                    },
                },
                // 同时保留 CPU 利用率指标
                {
                    Type: autoscalingv2.ResourceMetricSourceType,
                    Resource: &autoscalingv2.ResourceMetricSource{
                        Name: corev1.ResourceCPU,
                        Target: autoscalingv2.MetricTarget{
                            Type:               autoscalingv2.UtilizationMetricType,
                            AverageUtilization: ptr.To[int32](70),
                        },
                    },
                },
            },
        },
    }
    // ...
}
```

- [x] **Step 3: 验证**

```bash
# 在 envtest 环境中验证 HPA 创建
go test -tags=envtest -v -timeout=120s ./operator/controller/...
```

---

### Task 7: Pod Disruption Budget

**Files:**
- Create: `operator/controller/pdb.go`
- Modify: `operator/controller/agent_controller.go`

- [x] **Step 1: 创建 PDB**

```go
func (r *AgentDeploymentReconciler) ensurePDB(ctx context.Context, deploy *agentv1.AgentDeployment) error {
    pdb := &policyv1.PodDisruptionBudget{
        ObjectMeta: metav1.ObjectMeta{
            Name:      deploy.Name + "-pdb",
            Namespace: deploy.Namespace,
        },
        Spec: policyv1.PodDisruptionBudgetSpec{
            MinAvailable: intstr.FromInt(int(deploy.Spec.Replicas / 2)), // 至少保持 50% 可用
            Selector: &metav1.LabelSelector{
                MatchLabels: map[string]string{"app": deploy.Name},
            },
        },
    }
    // ...
}
```

- [x] **Step 2: 验证**

```bash
go test -tags=envtest -v ./operator/controller/ -run TestPDB
```

---

### Task 8: HPA 缩容稳定窗口

**Files:**
- Modify: `operator/controller/agent_controller.go`

- [x] **Step 1: 配置 HPA 行为**

```go
hpa.Spec.Behavior = &autoscalingv2.HorizontalPodAutoscalerBehavior{
    ScaleDown: &autoscalingv2.HPAScalingRules{
        StabilizationWindowSeconds: ptr.To[int32](300), // 5 分钟稳定窗口
        Policies: []autoscalingv2.HPAScalingPolicy{
            {
                Type:          autoscalingv2.PercentScalingPolicy,
                Value:         25, // 每次最多缩容 25%
                PeriodSeconds: 60,
            },
        },
    },
    ScaleUp: &autoscalingv2.HPAScalingRules{
        StabilizationWindowSeconds: ptr.To[int32](30),
        Policies: []autoscalingv2.HPAScalingPolicy{
            {
                Type:          autoscalingv2.PercentScalingPolicy,
                Value:         100, // 快速扩容
                PeriodSeconds: 30,
            },
        },
    },
}
```

---

### Task 9: Operator 滚动升级

**Files:**
- Modify: `operator/controller/agent_controller.go`

- [x] **Step 1: 支持滚动升级策略**

```go
// Deployment 更新策略
deployment.Spec.Strategy = appsv1.DeploymentStrategy{
    Type: appsv1.RollingUpdateDeploymentStrategyType,
    RollingUpdate: &appsv1.RollingUpdateDeployment{
        MaxUnavailable: intstr.FromInt(1),
        MaxSurge:       intstr.FromInt(1),
    },
}

// 升级时先启动新 Pod，健康检查通过后再终止旧 Pod
```

- [x] **Step 2: 优雅关闭集成**

确保 Agent Pod 在收到 SIGTERM 时：
1. 停止接受新任务
2. 等待正在执行的任务完成（最长 30s）
3. 保存 checkpoint
4. 退出

---

## Phase 4D：日志聚合增强（第 13-15 天）

### Task 10: 结构化日志标准化

**Files:**
- Modify: `internal/agent/react_loop_core.go`
- Modify: `internal/agent/react_loop_tools.go`
- Modify: `internal/llm/*_provider.go`
- Modify: `internal/pool/dispatcher.go`

- [x] **Step 1: 统一日志字段**

所有日志使用统一的字段名：

```go
// 统一字段名常量
const (
    FieldAgentID    = "agent_id"
    FieldSessionID  = "session_id"
    FieldTurn       = "turn"
    FieldProvider   = "provider"
    FieldModel      = "model"
    FieldTool       = "tool"
    FieldDuration   = "duration_ms"
    FieldError      = "error"
    FieldTraceID    = "trace_id"
    FieldSpanID     = "span_id"
)
```

- [x] **Step 2: 将 log.Default() 替换为 slog**

搜索所有 `log.Printf` / `log.Default()` 调用，替换为结构化 `slog`：

```bash
grep -rn "log.Printf\|log.Default()" internal/ --include="*.go" | grep -v _test.go
```

逐个替换为：
```go
slogger.Info("tool executed",
    "tool", tc.Name,
    "duration_ms", duration.Milliseconds(),
)
```

- [x] **Step 3: 日志关联 Trace ID**

```go
// 从 context 中提取 trace ID，写入日志
logger := slogger.With("trace_id", traceIDFromContext(ctx))
```

---

## 验收标准

1. `go build ./...` 和 `go vet ./...` 零错误
2. `go test -race -count=1 ./...` 全部通过
3. A2A gRPC 调用自动传播 W3C Trace Context（跨 Agent 追踪链路可见）
4. Pipeline/DAG/Handoff 每个节点自动创建子 span
5. LLM 调用 span 包含 provider/model/tokens/cost attribute
6. Grafana 有 6 个 Dashboard（agent/llm/cost/pool/memory/orchestration）
7. Prometheus 有 7+ 条告警规则覆盖 SLO
8. K8s Operator 支持基于 `concurrent_tasks_per_pod` 自定义指标 HPA
9. Pod Disruption Budget 保证滚动升级时 50% 可用
10. HPA 缩容有 5 分钟稳定窗口
11. 所有日志使用 slog 结构化输出，关联 trace ID
12. Operator envtest 测试通过

## 预期成果

| 指标 | 当前 | 目标 |
|------|------|------|
| Dashboard 数量 | 3 | 6 |
| 告警规则 | 0 | 7+ |
| 跨 Agent 追踪 | 不可见 | 完整调用链 |
| 编排层追踪 | 不可见 | 每节点 span |
| HPA 自定义指标 | 无 | concurrent_tasks_per_pod |
| PDB | 无 | 50% minAvailable |
| 结构化日志 | 部分 | 全部 slog + trace ID |
| 日志关联 Trace | 无 | 有 |
