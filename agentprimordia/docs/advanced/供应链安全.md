# 供应链安全加固指南

> 本文档描述 AgentPrimordia 项目的供应链安全策略和实践。

## 概述

AgentPrimordia 采用多层防御策略保护供应链安全：

```
┌──────────────────────────────────────────────────────┐
│                  供应链安全层级                        │
├──────────────────────────────────────────────────────┤
│  Layer 1: 依赖锁定与校验                               │
│  - Go Modules (go.sum 校验)                           │
│  - npm package-lock.json                             │
├──────────────────────────────────────────────────────┤
│  Layer 2: 漏洞扫描                                    │
│  - govulncheck (Go)                                  │
│  - npm audit (Node.js)                               │
│  - Socket.dev (npm 包行为分析)                        │
│  - Trivy (文件系统 + 容器镜像)                         │
├──────────────────────────────────────────────────────┤
│  Layer 3: PR 依赖审查                                 │
│  - GitHub Dependency Review Action                   │
│  - 许可证合规检查                                      │
├──────────────────────────────────────────────────────┤
│  Layer 4: 构建产物签名                                │
│  - cosign 签名二进制和容器镜像                         │
│  - SBOM (SPDX) 生成                                   │
├──────────────────────────────────────────────────────┤
│  Layer 5: 运行时安全                                  │
│  - Sandbox 命令白名单                                 │
│  - ACL 资源访问控制                                   │
│  - 输入校验 (Shell 元字符检测, 路径遍历防护)            │
└──────────────────────────────────────────────────────┘
```

## Go 供应链安全

### Go Modules 校验

Go Modules 内置完整性校验：

- `go.sum` 包含每个依赖的哈希值
- `go mod verify` 验证缓存的依赖未被篡改
- `GOFLAGS=-mod=readonly` 防止意外修改 `go.mod`

```bash
# 验证依赖完整性
go mod verify

# 检查依赖更新
go list -m -u all

# 审计依赖树
go mod why -m <module>
```

### govulncheck

CI 中已集成 `govulncheck`，扫描已知漏洞：

```bash
# 本地运行
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

### Go 许可证检查

CI 中使用 `go-licenses` 检查依赖许可证合规性：

```bash
go install github.com/google/go-licenses@latest
go-licenses check ./...
```

项目使用 Apache-2.0 许可证，拒绝 GPL/AGPL/LGPL 等不兼容许可证。

## TypeScript 供应链安全

### package-lock.json

`package-lock.json` 锁定所有依赖的确切版本和哈希值：

```bash
# 验证锁定文件一致性
npm ci  # 严格按照 lockfile 安装
```

### npm audit

```bash
# 检查已知漏洞
npm audit

# 严格模式（high 及以上级别视为错误）
npm audit --audit-level=high
```

### Socket.dev 集成

[Socket.dev](https://socket.dev/) 提供比 npm audit 更深层的分析：
- 检测 install 脚本中的恶意行为
- 识别 typosquatting（仿冒包名）
- 监控依赖网络行为

CI 中已集成 Socket 扫描（需配置 `SOCKET_API_KEY` 密钥）：

```bash
npx @socketsecurity/cli scan --strict
```

> **配置 Socket API Key**：在 GitHub 仓库 Settings → Secrets → Actions 中添加 `SOCKET_API_KEY`。

## GitHub Dependency Review

PR 中自动运行 `dependency-review-action`，审查新增依赖：

- 拒绝 moderate 及以上级别的漏洞
- 拒绝 GPL-3.0、AGPL-3.0、LGPL-3.0 许可证
- 在 PR 中发布审查摘要

## 构建产物签名

### 二进制签名

Release 流程使用 cosign 签名所有构建产物：

1. 生成 SHA256 校验和
2. 使用 cosign 签名校验和文件
3. 上传签名和证书

验证签名：

```bash
# 下载 checksums-sha256.txt, checksums-sha256.txt.sig, checksums-sha256.txt.pem
cosign verify-blob \
  --certificate checksums-sha256.txt.pem \
  --signature checksums-sha256.txt.sig \
  --certificate-identity-regexp '.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  checksums-sha256.txt

# 验证后检查校验和
sha256sum -c checksums-sha256.txt
```

### 容器镜像签名

Docker 镜像使用 cosign 签名：

```bash
# 验证容器镜像签名
cosign verify ghcr.io/agentprimordia/ap:latest \
  --certificate-identity-regexp '.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
```

### SBOM (Software Bill of Materials)

每次 Release 生成 SPDX 格式的 SBOM：

```bash
# SBOM 包含所有依赖的完整清单
# 可用于追踪漏洞影响范围
ls sbom-*.spdx.json
```

## 运行时安全

### 命令沙箱

```go
// 命令白名单
sb := security.NewSandbox(acl)
sb.AllowCommand("git")
sb.AllowCommand("go")

// 参数白名单模式
sb.AllowCommandWithArgs("git",
    security.NewArgPattern(`^commit -m "[\w\s]+"$`, "仅允许 commit -m"))

// 路径访问控制
acl := security.NewACL()
acl.Allow("agent-1", "/data/", security.AccessRead|security.AccessWrite)
acl.Deny("agent-1", "/etc/")
```

### 输入校验

```go
// Shell 元字符检测
if hasMeta, ch := security.ContainsShellMetacharacter(input); hasMeta {
    return fmt.Errorf("输入包含危险字符: %s", ch)
}

// 路径遍历防护
if err := sb.ValidatePath("agent-1", userInput, security.AccessRead); err != nil {
    return err
}
```

## 安全事件响应

### 发现漏洞时的流程

1. **确认**：使用 `govulncheck` / `npm audit` 确认漏洞影响范围
2. **评估**：根据 CVSS 评分评估严重程度
3. **修复**：升级到修复版本，或应用补丁
4. **验证**：运行测试套件确认修复有效
5. **发布**：发布安全版本，更新 CHANGELOG
6. **通知**：通过 GitHub Security Advisories 通知用户

### 报告安全漏洞

请勿通过公开 Issue 报告安全漏洞。使用 GitHub Security Advisories：

1. 前往仓库 Security → Advisories
2. 点击 "Report a vulnerability"
3. 填写漏洞详情和影响范围

## 安全检查清单

### 开发时

- [ ] `go mod verify` 通过
- [ ] `npm ci` 一致性检查通过
- [ ] 无新的 `install` 脚本依赖
- [ ] 新增依赖许可证兼容

### CI/CD

- [ ] govulncheck 无 HIGH/CRITICAL 漏洞
- [ ] npm audit 无 HIGH 及以上漏洞
- [ ] Socket 扫描无告警（如已配置）
- [ ] Dependency Review 通过
- [ ] Trivy 扫描无 CRITICAL 漏洞

### Release

- [ ] SBOM 已生成
- [ ] 二进制已 cosign 签名
- [ ] 容器镜像已 cosign 签名
- [ ] SHA256 校验和已发布

## 参考

- [Go Security](https://go.dev/security/)
- [npm Security](https://docs.npmjs.com/about-npm-security)
- [Socket.dev](https://socket.dev/)
- [SLSA Framework](https://slsa.dev/)
- [cosign](https://github.com/sigstore/cosign)
