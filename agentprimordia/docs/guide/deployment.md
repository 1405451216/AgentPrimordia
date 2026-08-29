# 部署与运维

> 从开发到生产的完整部署指南。

## 单机部署

### 二进制

```bash
# 交叉编译（Linux）
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ap ./cmd/ap/

scp ap user@server:~/
ssh user@server 'AP_LLM_API_KEY=sk-xxx ./ap run'
```

### Docker

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -o ap ./cmd/ap/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/ap /usr/local/bin/
EXPOSE 8080
# --watch: 监听项目文件（含 .ap.yaml）变更，自动重新编译并重启 Agent（热重载）
CMD ["ap", "run", "--watch"]
```

```bash
docker build -t agentprimordia:latest .
docker run -e AP_LLM_API_KEY=sk-xxx -p 8080:8080 agentprimordia:latest
```

## Kubernetes 部署

```bash
# 安装 Operator
kubectl apply -f https://agentprimordia.dev/operator/install.yaml

# 创建 AgentDeployment
kubectl apply -f agent-deployment.yaml
```

## 配置热加载

配置热重载由 `ap run --watch` 提供：它会监听项目目录的文件变更（包括
`.ap.yaml`），检测到变更后自动重新编译并重启 Agent，使配置修改即时生效，
无需手动重启进程：

```bash
ap run --watch
```

说明：pkg 公共 API 提供的配置加载器（`ap.NewConfigLoader`，YAML < ENV <
flags）为一次性加载，不提供运行时 Watch / OnChange 回调；进程级热重载
统一由 CLI 的 `--watch` 模式完成。

## 监控

### Prometheus 指标

Admin HTTP 服务默认暴露 `/metrics`：

```yaml
admin:
  enabled: true
  addr: ":8080"
  metrics_path: "/metrics"
  pprof_enabled: true   # /debug/pprof/
```

### 健康检查

```
GET /healthz    → 200 OK
GET /readyz     → 200 (依赖就绪) / 503 (依赖异常)
```

### Grafana Dashboard

开箱即用的 Dashboard JSON 位于 `deploy/grafana/` 目录，包括
`agentprimordia-overview.json`（总览）、`dashboard-agent.json`（Agent）、
`dashboard-llm.json`（LLM）等，直接在 Grafana 中导入即可。

## 日志

支持 slog（结构化日志），输出到 stderr 或文件：

```yaml
logging:
  level: info                # debug / info / warn / error
  format: json              # json / text
  output: stderr            # stderr / stdout / file path
  sampling:
    rate: 0.1               # 高频日志采样率
```

## 安全加固

```yaml
security:
  tls:
    cert_file: /etc/tls/server.crt
    key_file:  /etc/tls/server.key
  cors:
    allowed_origins: ["https://app.example.com"]
  rate_limit:
    requests_per_second: 100
    burst: 200
```
