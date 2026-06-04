# AgentPrimordia K8s Operator

声明式 Agent 部署，通过 `AgentDeployment` CRD 管理。

## 前置条件

- Kubernetes 1.28+
- kubectl 访问集群

## 安装

```bash
# 安装 CRD
kubectl apply -f manifest/crd.yaml

# 部署 Operator
kubectl apply -f manifest/controller.yaml
```

## 使用

```bash
# 部署 Agent
kubectl apply -f manifest/examples/basic-agent.yaml

# 查看状态
kubectl get agentdeployments
kubectl describe ad basic-agent

# 扩容
kubectl scale ad code-reviewer --replicas=5
```

## 构建

```bash
cd operator
go mod tidy
go build -o ap-operator ./cmd/
```

## CRD 字段 (Phase 8.3 扩展)

`AgentDeploymentSpec.Template` 新增两个能力字段:

### 指标暴露 (Metrics)

```yaml
spec:
  template:
    metrics:
      enabled: true
      path: /metrics        # 默认 /metrics
      port: 9090            # 默认 9090
      serviceMonitor:       # 对接 Prometheus Operator
        name: agent-monitor
        interval: 30s
```

启用后 Operator 会为 Agent Pod 注入:
- 端口 9090 上的 `/metrics` HTTP 端点
- 配套 Service + ServiceMonitor 资源(若 ServiceMonitor 字段设置)

### 分布式追踪 (Tracing)

```yaml
spec:
  template:
    tracing:
      enabled: true
      otlpEndpoint: http://otel-collector:4317
      samplingRate: 0.1   # 10% 采样
```

启用后 Operator 注入 OTEL_EXPORTER_OTLP_ENDPOINT 环境变量
到 Agent Pod,框架内的 Tracer 会自动连接。

### Status 字段 (观测)

```yaml
status:
  activeReplicas: 3
  completedTasks: 1000
  failedTasks: 5
  errorRate: 0.005
  averageTurnLatencySeconds: 1.2
  totalTokens: 50000
  estimatedCostUSD: 1.5
  conditions:
    - type: Ready
      status: "True"
      reason: AllReplicasReady
```

Reconciler 每 30s 收集指标并更新 status (Phase 8.3 范围)。

## 已知债务

- `cmd/main.go` 用了 controller-runtime 0.20 已移除的 `MetricsBindAddress` 字段,
  需在 Phase 9 适配(改用 `metricsserver.Options`)
- `zz_generated_deepcopy.go` 是手写的,理想应改回 controller-gen codegen
- controller 单元测试覆盖率低,Phase 9+ 补


## 独立依赖说明

此目录有独立的 `go.mod`，包含 K8s 客户端依赖（k8s.io/api, k8s.io/client-go, controller-runtime），不污染主项目的零外部依赖约束。
