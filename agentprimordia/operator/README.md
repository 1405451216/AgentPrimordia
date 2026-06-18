# AgentPrimordia K8s Operator

声明式 Agent 部署，通过 `AgentDeployment` CRD 管理。

## 前置条件

- Kubernetes 1.28+
- kubectl 访问集群
- Go 1.26+ (构建)

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
GOWORK=off go mod tidy
GOWORK=off go build -o ap-operator ./cmd/
```

## 测试

### 单元测试

```bash
cd operator
GOWORK=off go test ./api/v1/ ./controller/ -count=1 -v
```

### 端到端测试 (envtest)

envtest 使用真实的 kube-apiserver 和 etcd，无需完整集群：

```bash
# 安装 setup-envtest
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# 下载 K8s 二进制
setup-envtest use 1.28.x

# 运行 e2e 测试
cd operator
KUBEBUILDER_ASSETS=$(setup-envtest use 1.28.x -p path) \
  GOWORK=off go test -tags=envtest ./controller/ -count=1 -v
```

### Controller 代码生成

```bash
cd operator
make codegen
# 或手动: GOWORK=off controller-gen object paths=./api/v1/...
```

## CRD 字段

### 基础字段

```yaml
apiVersion: agent.primordia.dev/v1
kind: AgentDeployment
metadata:
  name: basic-agent
spec:
  replicas: 1
  template:
    provider: openai
    model: gpt-4o
    systemPrompt: "你是一个有用的 AI 助手"
    maxTurns: 10
    apiSecretRef: openai-api-key
    tools:
      - name: filesystem
        config:
          root: /data
    memory:
      backend: sqlite
      sizeLimit: 1Gi
    resources:
      requests:
        cpu: "500m"
        memory: "256Mi"
      limits:
        cpu: "2"
        memory: "1Gi"
```

### 自动扩缩容

```yaml
spec:
  autoscaling:
    minReplicas: 1
    maxReplicas: 10
    targetConcurrentTasks: 5
```

### 健康检查

```yaml
spec:
  healthCheck:
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8080
      initialDelaySeconds: 10
      periodSeconds: 30
    readinessProbe:
      httpGet:
        path: /readyz
        port: 8080
      initialDelaySeconds: 5
      periodSeconds: 10
```

### 指标暴露 (Metrics)

```yaml
spec:
  template:
    metrics:
      enabled: true
      path: /metrics
      port: 9090
      serviceMonitor:
        name: agent-monitor
        interval: 30s
```

### 分布式追踪 (Tracing)

```yaml
spec:
  template:
    tracing:
      enabled: true
      otlpEndpoint: http://otel-collector:4317
      samplingRate: 0.1
```

### Status 字段

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
    - type: Available
      status: "True"
      reason: MinimumReplicasAvailable
```

## RBAC

Operator 需要以下权限：

```yaml
# AgentDeployment CRD 读写
- apiGroups: ["agent.primordia.dev"]
  resources: ["agentdeployments", "agentdeployments/status"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]

# Deployment 管理
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# ConfigMap 管理
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Secret 读取
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]
```

## 独立依赖说明

此目录有独立的 `go.mod`，包含 K8s 客户端依赖（k8s.io/api, k8s.io/client-go, controller-runtime），不污染主项目的零外部依赖约束。
