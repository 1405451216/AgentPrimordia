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

## 独立依赖说明

此目录有独立的 `go.mod`，包含 K8s 客户端依赖（k8s.io/api, k8s.io/client-go, controller-runtime），不污染主项目的零外部依赖约束。
