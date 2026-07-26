# 集群多节点生产化验证方案

> **状态**: Draft (v3.2 P1)
> **目标**: 验证 `internal/agent/cluster/` 在真实多节点环境下的稳定性与分区容错

## 1. 验证环境

### 1.1 Docker Compose 拓扑

```yaml
# deploy/compose/cluster-e2e.yml
version: "3.8"
services:
  etcd:
    image: quay.io/coreos/etcd:v3.5.12
    command: >
      etcd
      --name etcd0
      --advertise-client-urls http://0.0.0.0:2379
      --listen-client-urls http://0.0.0.0:2379
      --initial-advertise-peer-urls http://0.0.0.0:2380
      --listen-peer-urls http://0.0.0.0:2380
    ports:
      - "2379:2379"

  node-1:
    build: ../..
    environment:
      - AP_NODE_ID=node-1
      - AP_ETCD_ENDPOINTS=etcd:2379
      - AP_GRPC_PORT=9001
    depends_on: [etcd]

  node-2:
    build: ../..
    environment:
      - AP_NODE_ID=node-2
      - AP_ETCD_ENDPOINTS=etcd:2379
      - AP_GRPC_PORT=9002
    depends_on: [etcd]

  node-3:
    build: ../..
    environment:
      - AP_NODE_ID=node-3
      - AP_ETCD_ENDPOINTS=etcd:2379
      - AP_GRPC_PORT=9003
    depends_on: [etcd]
```

### 1.2 启动命令

```bash
cd deploy/compose
docker compose -f cluster-e2e.yml up -d
```

## 2. 验证矩阵

| # | 验证项 | 方法 | 通过标准 |
|---|--------|------|----------|
| 1 | 节点注册 | 3 节点启动后查询 etcd | 3 个 Lease 均存在 |
| 2 | 互相发现 | 每个节点 ListNodes() | 返回 3 个节点 |
| 3 | 消息路由 | node-1 → node-3 发送消息 | node-3 收到且内容正确 |
| 4 | 节点下线 | 停止 node-2 | 其余节点 30s 内感知（Watch） |
| 5 | 节点恢复 | 重启 node-2 | 重新注册，恢复消息路由 |
| 6 | 网络分区 | iptables 隔离 node-3 | 分区两侧各自可用 |
| 7 | 分区恢复 | 移除 iptables 规则 | 自动重新连接 |
| 8 | 高并发 | 100 并发跨节点消息 | 无丢失、延迟 < 100ms P99 |
| 9 | Leader 选举 | 模拟 Leader 宕机 | 新 Leader 在 10s 内选出 |
| 10 | 数据一致性 | 并发写入后全量读取 | 所有节点数据一致 |

## 3. 自动化测试脚本

```bash
#!/bin/bash
# scripts/cluster-e2e.sh — 集群 E2E 验证自动化

set -euo pipefail

COMPOSE_FILE="deploy/compose/cluster-e2e.yml"
TIMEOUT=120

echo "=== AgentPrimordia 集群 E2E 验证 ==="
echo "启动时间: $(date)"

# 1. 启动环境
echo "[1/6] 启动 Docker Compose 环境..."
docker compose -f $COMPOSE_FILE up -d --build
sleep 10  # 等待 etcd 就绪

# 2. 验证节点注册
echo "[2/6] 验证节点注册..."
for node in node-1 node-2 node-3; do
  docker compose -f $COMPOSE_FILE exec $node \
    ap cluster status --format json | jq '.nodes | length'
done

# 3. 跨节点消息
echo "[3/6] 验证跨节点消息路由..."
docker compose -f $COMPOSE_FILE exec node-1 \
  ap cluster send --target node-3 --message "hello-from-1"

# 4. 节点下线感知
echo "[4/6] 验证节点下线感知..."
docker compose -f $COMPOSE_FILE stop node-2
sleep 35  # 等待 Lease 过期
docker compose -f $COMPOSE_FILE exec node-1 \
  ap cluster status --format json | jq '.nodes | length'
# 期望输出: 2

# 5. 节点恢复
echo "[5/6] 验证节点恢复..."
docker compose -f $COMPOSE_FILE start node-2
sleep 10
docker compose -f $COMPOSE_FILE exec node-1 \
  ap cluster status --format json | jq '.nodes | length'
# 期望输出: 3

# 6. 清理
echo "[6/6] 清理环境..."
docker compose -f $COMPOSE_FILE down -v

echo "=== 验证完成: $(date) ==="
```

## 4. K8s 环境验证（进阶）

对于 K8s 环境，使用 Operator 部署：

```bash
# 部署 3 副本 AgentDeployment
kubectl apply -f - <<EOF
apiVersion: agentprimordia.io/v1
kind: AgentDeployment
metadata:
  name: cluster-e2e
spec:
  replicas: 3
  template:
    spec:
      env:
        - name: AP_ETCD_ENDPOINTS
          value: "etcd.default.svc:2379"
EOF

# 验证 Pod 状态
kubectl get pods -l app=cluster-e2e

# 验证集群发现
kubectl exec -it cluster-e2e-0 -- ap cluster status
```

## 5. 性能基线

| 指标 | 目标值 | 测量方法 |
|------|--------|----------|
| 节点发现延迟 | < 5s | etcd Watch 事件时间戳 |
| 跨节点消息 P99 | < 100ms | gRPC 往返时间 |
| 故障感知时间 | < 30s | Lease TTL（默认 10s + KeepAlive 间隔） |
| 分区恢复时间 | < 10s | 网络恢复后重连时间 |
| 并发消息吞吐 | > 1000 msg/s | 压测工具 |

## 6. 已知限制

- etcd 集群发现依赖 build tag `etcd`，默认构建不包含
- gRPC 消息总线复用 A2A 基础设施，需要 protobuf 编译
- 网络分区测试需要 Linux + iptables 权限
- Windows/macOS 环境仅支持 Docker Desktop 方式验证
