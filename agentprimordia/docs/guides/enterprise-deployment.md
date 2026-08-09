# 企业参考部署（v4.8-5）

> 目标：Helm/Terraform 一键部署 + 案例文档。

## 部署资产

| 资产 | 位置 | 说明 |
|------|------|------|
| Docker Compose（开发/演示） | `deploy/compose/` | 单机全能力（API + Studio + 可选 etcd/redis） |
| Docker Compose（分布式测试） | `deploy/compose/distributed-test.yaml` | etcd + redis 测试依赖 |
| Helm Chart | `deploy/helm/` | Kubernetes 生产形态 |
| Terraform | `deploy/terraform/` | 云资源（VPC/节点/存储） |
| 系统服务 | `deploy/autonomous-agent.service` | 单机自治 Agent systemd |
| Operator CRD | `operator/` | AgentDeployment 自定义资源 |

## 一键部署案例：自治数据修复 Agent（K8s）

```bash
# 1. 基础设施（Terraform）
cd deploy/terraform && terraform init && terraform apply -auto-approve

# 2. 应用（Helm）
helm install ap deploy/helm/ \
  --set llm.provider=openai \
  --set llm.apiKeySecret=ap-llm-key \
  --set autonomy.enabled=true \
  --set skills.enabled=true \
  --set a2a.enabled=true \
  --set realtime.enabled=true \
  --set studio.enabled=true

# 3. 验证
kubectl port-forward svc/ap-studio 8090:8090
# 打开 http://localhost:8090（Studio 面板显示真实运行数据）
```

## 单机部署案例：边缘监控 Agent（systemd）

```bash
# 构建
cd agentprimordia && go build -o /usr/local/bin/ap-agent ./cmd/ap

# 服务（deploy/autonomous-agent.service 示例）
sudo cp deploy/autonomous-agent.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now ap-agent
journalctl -u ap-agent -f   # 自治目标执行日志
```

## 生产形态检查清单

- [ ] 密钥走 `SecretsManager`（Vault 后端）而非明文环境变量
- [ ] 检查点/失败记录持久化（SQLite 文件挂持久卷，或 etcd/redis 后端）
- [ ] 多租户：过滤器级（默认）或物理分库（`NewPhysicalTenantStore`）
- [ ] 护栏默认开启（输入/输出 + 敏感工具审计）
- [ ] 可观测：OTel 导出 + `/api/failures` 失败重放
- [ ] 供应链：SBOM 签名产物 + govulncheck 门
- [ ] 成本护栏：目标级预算 + 租户配额（v4.9-4）
