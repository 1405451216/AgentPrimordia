# Supply Chain Security（供应链安全）

本文档描述 AgentPrimordia 项目的供应链安全实践，包括 SBOM 生成、镜像签名和合规扫描。

## 概述

AgentPrimordia 遵循 [SLSA](https://slsa.dev/) 框架和 [Supply Chain Levels for Software Artifacts](https://slsa.dev/spec/v1.0/levels) 的要求，确保从源代码到容器镜像的每个环节可追溯、可验证。

## SBOM（Software Bill of Materials）

### 什么是 SBOM？

SBOM 是软件组件清单，列出构成软件的所有依赖项、库及其版本信息。AgentPrimordia 使用 [CycloneDX](https://cyclonedx.org/) 格式。

### 生成 SBOM

```bash
# 本地生成
./scripts/generate-sbom.sh sbom.json cyclonedx-json

# 指定输出格式
./scripts/generate-sbom.sh sbom.spdx spdx-json
```

### 依赖工具

- [Syft](https://github.com/anchore/syft) — SBOM 生成器

```bash
# macOS
brew install syft

# Linux
curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin
```

## 容器镜像签名（Cosign）

### 使用方式

```bash
# Keyless 签名（推荐，使用 Fulcio + Rekor）
./scripts/cosign-sign.sh ghcr.io/agentprimordia/app:latest

# 密钥签名
COSIGN_PRIVATE_KEY=$(cat cosign.key) ./scripts/cosign-sign.sh ghcr.io/agentprimordia/app:latest
```

### 验证签名

```bash
cosign verify ghcr.io/agentprimordia/app:latest
```

### 依赖工具

- [Cosign](https://github.com/sigstore/cosign) — 容器镜像签名

```bash
# macOS
brew install cosign

# Linux
curl -sL https://github.com/sigstore/cosign/releases/latest/download/cosign-linux-amd64 -o cosign
chmod +x cosign && mv cosign /usr/local/bin/
```

## CI/CD 集成

### GitHub Actions Workflow

`.github/workflows/supply-chain.yml` 包含三个阶段：

| 阶段 | 说明 |
|------|------|
| **SBOM** | 使用 Syft 生成 CycloneDX SBOM 并上传为 artifact |
| **Build & Sign** | 构建容器镜像，使用 Cosign keyless 签名 |
| **Vulnerability Scan** | 使用 Trivy 扫描依赖漏洞 |

### 触发条件

- 推送到 `main` 或 `release/*` 分支
- 针对 `main` 的 Pull Request
- 手动触发（`workflow_dispatch`）

## 安全标签说明

Dockerfile.sbomlabel 中使用的 OCI 标签：

| 标签 | 说明 |
|------|------|
| `org.opencontainers.image.title` | 镜像名称 |
| `org.opencontainers.image.version` | 版本号 |
| `org.opencontainers.image.revision` | Git commit SHA |
| `com.agentprimordia.sbom` | SBOM 文件路径 |
| `com.agentprimordia.sbom.format` | SBOM 格式 |

## 合规标准

- [SLSA v1.0](https://slsa.dev/spec/v1.0/levels) — 供应链完整性框架
- [CycloneDX v1.6](https://cyclonedx.org/specification/overview/) — SBOM 标准格式
- [OCI Image Spec](https://github.com/opencontainers/image-spec) — 容器镜像标签规范
