# 混沌工程 E2E 测试运行指南

本文档描述如何在特权容器中运行 AgentPrimordia 混沌工程端到端测试。

## 前置条件

- Docker 环境
- Linux 内核（网络分区测试需要 `CAP_NET_ADMIN` 能力）
- Go 1.21+

## 快速运行（本地）

### LLM Provider 故障转移（无需特权）

```bash
cd agentprimordia
go test -tags=e2e -run TestE2E_Chaos_LLMProviderFailover -v ./internal/chaos/
```

### 网络分区注入（需要 Linux + root）

```bash
sudo go test -tags=e2e -run TestE2E_Chaos_NetworkPartition -v ./internal/chaos/
```

### Soak + Chaos 联动测试

```bash
# 默认 10 秒快速验证
go test -tags=e2e -run TestE2E_Chaos_SoakIntegration -v ./internal/chaos/

# 自定义持续时间（30 秒）
CHAOS_SOAK_DURATION=30 go test -tags=e2e -run TestE2E_Chaos_SoakIntegration -v ./internal/chaos/
```

## Docker 特权容器运行

### 构建测试镜像

```dockerfile
FROM golang:1.22-bookworm

WORKDIR /workspace
COPY . .

# 安装网络故障注入工具
RUN apt-get update && apt-get install -y iproute2 iptables && rm -rf /var/lib/apt/lists/*

CMD ["go", "test", "-tags=e2e", "-v", "-timeout=30m", "./internal/chaos/..."]
```

### 运行特权容器

```bash
docker build -t ap-chaos-e2e -f Dockerfile.chaos .

docker run --rm \
  --privileged \
  --cap-add=NET_ADMIN \
  --cap-add=SYS_ADMIN \
  -e CHAOS_SOAK_DURATION=60 \
  ap-chaos-e2e
```

### 仅运行特定测试

```bash
# 仅 LLM 故障转移
docker run --rm ap-chaos-e2e \
  go test -tags=e2e -run TestE2E_Chaos_LLMProviderFailover -v ./internal/chaos/

# 仅网络分区（需要特权）
docker run --rm --privileged --cap-add=NET_ADMIN ap-chaos-e2e \
  go test -tags=e2e -run TestE2E_Chaos_NetworkPartition -v ./internal/chaos/
```

## CI/CD 集成

### GitHub Actions 示例

```yaml
chaos-e2e:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
      with:
        go-version: '1.22'
    - name: Run Chaos E2E
      working-directory: agentprimordia
      env:
        CHAOS_SOAK_DURATION: "30"
      run: |
        sudo go test -tags=e2e \
          -run TestE2E_Chaos \
          -v -timeout=10m \
          ./internal/chaos/...
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `CHAOS_SOAK_DURATION` | `10` | Soak+Chaos 联动测试持续时间（秒） |

## 测试说明

| 测试名称 | 环境要求 | 说明 |
|----------|----------|------|
| `TestE2E_Chaos_LLMProviderFailover` | 无 | 验证 LLM 503/429 故障注入与恢复 |
| `TestE2E_Chaos_NetworkPartition` | Linux + root | 真实 iptables/tc 网络故障注入 |
| `TestE2E_Chaos_SoakIntegration` | 无 | Soak 负载 + 混沌实验联动 |

## 故障排查

- **测试被 Skip**：检查是否满足环境要求（Linux/root），测试会自动跳过不满足条件的用例
- **端口冲突**：LLM 故障测试使用 18995-18999 端口，确保未被占用
- **超时**：Soak 测试默认 10 秒，CI 中建议设置 `CHAOS_SOAK_DURATION=30`
