# AgentPrimordia Grafana Dashboard

一键导入 AgentPrimordia 监控面板到 Grafana。

## 快速开始

### 方式一：直接导入 JSON

1. 打开 Grafana → Dashboards → Import
2. 上传 JSON 文件或粘贴内容
3. 选择 Prometheus 数据源
4. 点击 Import

### 方式二：K8s ConfigMap 自动挂载

```bash
kubectl create configmap ap-dashboard-agent \
  --from-file=dashboard-agent.json \
  -n monitoring

kubectl create configmap ap-dashboard-llm \
  --from-file=dashboard-llm.json \
  -n monitoring

kubectl create configmap ap-dashboard-cost \
  --from-file=dashboard-cost.json \
  -n monitoring
```

在 Grafana 的 `grafana.ini` 或 sidecar 配置中设置 dashboard provider 指向这些 ConfigMap。

### 方式三：Grafana Provisioning

将 `datasource.yml` 和 dashboard JSON 放入 Grafana 的 provisioning 目录：

```bash
cp datasource.yml /etc/grafana/provisioning/datasources/
cp dashboard-*.json /etc/grafana/provisioning/dashboards/
```

## Dashboard 说明

| Dashboard | 文件 | 内容 |
|-----------|------|------|
| Agent Runtime | `dashboard-agent.json` | 活跃agent数、轮次延迟、工具调用频率、错误率 |
| LLM Operations | `dashboard-llm.json` | LLM延迟P50/P95/P99、Token消耗、Provider分布 |
| Cost Tracking | `dashboard-cost.json` | 成本趋势、按Provider/Agent/Model成本分解 |

## 前置条件

- Prometheus 已配置抓取 AgentPrimordia 的 `/metrics` 端点
- AP 应用已启用 `internal/metrics` 模块
- 推荐 scrape interval: 10s

## Prometheus 抓取配置示例

```yaml
scrape_configs:
  - job_name: 'agentprimordia'
    scrape_interval: 10s
    static_configs:
      - targets: ['agent:8080']
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'ap_.*'
        action: keep
```
