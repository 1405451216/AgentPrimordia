# 阶段五：生态建设与社区实施计划（6-12 周）

> **状态：部分进行中 🟡**（Task 1 已完成 80%：install/list/create/remove/search/update 全部落地 + 17 个测试通过）
> **创建日期：2026-07-05**
> **前置文档**：`docs/plans/2026-06-22-long-term-vision.md`（长期愿景 Phase 9）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## 目标

构建插件市场与注册中心、企业版多租户隔离、TypeScript SDK 边缘运行时扩展、社区运营工具链，使 AgentPrimordia 从框架演进为平台级生态。

## 当前状态盘点

| 组件 | 状态 | 说明 |
|------|------|------|
| 插件加载器 (`ecosystem/plugins/loader.go`) | ✅ 完成 | Discover/Load/Validate，支持 plugin.json 清单 |
| 基础插件 | ✅ 完成 | 6 个：email/git/http/json/kv/sql |
| 插件注册表 (`registry.json`) | ✅ 完成 | 本地注册表 |
| CLI 插件命令 | ⬜ 未开始 | 无 `ap plugin install/list/search` |
| 插件签名验证 | ⬜ 未开始 | 无 cosign 签名验证 |
| 插件沙箱 | ⬜ 未开始 | 插件运行在主进程内，无隔离 |
| 多租户 | ⬜ 未开始 | 无租户隔离 |
| TS SDK Edge Runtime | ⚠️ 部分 | `src/edge/` 有 cold-start.ts 和 runtime.ts，未完整 |
| React Hooks | ⬜ 未开始 | 无 React 绑定 |
| VS Code 扩展 | ⬜ 未开始 | 无 |
| 社区工具链 | ⬜ 未开始 | 无贡献者指南 / 模板 / CI 脚手架 |

---

## Phase 5A：插件市场与注册中心（第 1-3 周）

### Task 1: CLI 插件管理命令

**Files:**
- Create: `cmd/ap/plugin.go`
- Create: `cmd/ap/plugin_test.go`

- [x] **Step 1: 实现 `ap plugin install`**

```go
// cmd/ap/plugin.go（switch-case 风格，与现有 init/run/mcp 等命令保持一致）
func runPlugin(args []string) error { /* ... */ }
func pluginInstall(args []string) error { /* go get + 写 .ap.yaml */ }

// ap plugin install github-issues
// 1. 调用 `go get <module>` 拉取依赖
// 2. 更新 .ap.yaml 的 plugins 列表
// 3. （TODO: Task 3 完成后）验证签名
// 4. （TODO: Task 2 完成后）从远程注册中心查询元数据
```

- [x] **Step 2: 实现 `ap plugin list`**

```go
// ap plugin list — 列出 .ap.yaml 中已安装的插件
// ap plugin search --installed — 列出本地注册表中已安装的插件（带 status）
```

- [x] **Step 3: 实现 `ap plugin search`**

```go
// ap plugin search database — 按关键词搜索
// ap plugin search --category=vcs — 按分类过滤
// ap plugin search --installed — 仅列出已安装
// 搜索来源：ecosystem/plugins/registry.json（本地）/ $HOME/.agentprimordia/plugins/registry.json（用户级）
```

- [x] **Step 4: 编写测试**（cmd/ap/plugin_test.go 共 17 个 TestPlugin / TestRunPlugin / TestLoadPluginRegistry / TestMatchPlugin / TestIsInstalled 测试）

```go
func TestPluginSearch_ByKeyword(t *testing.T) { /* ... */ }
func TestPluginSearch_ByCategory(t *testing.T) { /* ... */ }
func TestPluginSearch_InstalledOnly(t *testing.T) { /* ... */ }
func TestLoadPluginRegistry_ValidFile(t *testing.T) { /* ... */ }
```

- [x] **Step 5: 验证** ✅（`go test ./cmd/ap/` 全绿；新增 pluginSearch / pluginUpdate / loadPluginRegistry / isInstalled / matchPluginEntry 17 个测试）

```bash
go test -v ./cmd/ap/ -run TestPlugin  # ok
```

> **遗留**：Task 2/3 依赖项（远程注册中心、cosign 签名）尚未实现，本 Task 的 `install` 暂未集成签名校验。

---

### Task 2: 远程插件注册中心

**Files:**
- Create: `internal/registry/registry.go`
- Create: `internal/registry/registry_test.go`
- Create: `cmd/ap/registry_server.go`（注册中心服务端）

- [ ] **Step 1: 定义注册中心 API**

```go
// internal/registry/registry.go
package registry

// RegistryClient 注册中心客户端
type RegistryClient struct {
    baseURL    string
    httpClient *http.Client
}

// Search 搜索插件
func (c *RegistryClient) Search(query string) ([]PluginInfo, error)

// Get 获取插件详情
func (c *RegistryClient) Get(name string) (*PluginInfo, error)

// Download 下载插件包
func (c *RegistryClient) Download(name, version string) (io.ReadCloser, error)

// Publish 发布插件到注册中心
func (c *RegistryClient) Publish(plugin *PluginInfo, token string) error

// PluginInfo 插件完整信息
type PluginInfo struct {
    Name         string            `json:"name"`
    Version      string            `json:"version"`
    Type         string            `json:"type"`
    Description  string            `json:"description"`
    Author       string            `json:"author"`
    Homepage     string            `json:"homepage"`
    Repository   string            `json:"repository"`
    License      string            `json:"license"`
    Checksum     string            `json:"checksum"`    // SHA256
    Signature    string            `json:"signature"`   // cosign 签名
    Dependencies []string          `json:"dependencies"`
    Config       map[string]string `json:"config"`
    CreatedAt    time.Time         `json:"created_at"`
    UpdatedAt    time.Time         `json:"updated_at"`
    Downloads    int64             `json:"downloads"`
    Rating       float64           `json:"rating"`
}
```

- [ ] **Step 2: 实现注册中心服务端**

```go
// cmd/ap/registry_server.go
// 基于 net/http 的轻量注册中心
// GET  /api/v1/plugins            — 搜索
// GET  /api/v1/plugins/{name}     — 详情
// GET  /api/v1/plugins/{name}/{version}/download — 下载
// POST /api/v1/plugins             — 发布（需认证）
```

- [ ] **Step 3: 编写测试**

```go
func TestRegistry_Search(t *testing.T) { /* ... */ }
func TestRegistry_Download(t *testing.T) { /* ... */ }
func TestRegistry_Publish(t *testing.T) { /* ... */ }
```

---

### Task 3: 插件签名验证

**Files:**
- Create: `internal/registry/signing.go`
- Create: `internal/registry/signing_test.go`

- [ ] **Step 1: 实现 cosign 签名验证**

```go
// internal/registry/signing.go
package registry

import (
    "crypto/ed25519"
    "crypto/sha256"
)

// VerifySignature 验证插件包签名
func VerifySignature(data []byte, signature []byte, publicKey ed25519.PublicKey) bool {
    hash := sha256.Sum256(data)
    return ed25519.Verify(publicKey, hash[:], signature)
}

// SignPlugin 为插件包签名
func SignPlugin(data []byte, privateKey ed25519.PrivateKey) ([]byte, error) {
    hash := sha256.Sum256(data)
    return ed25519.Sign(privateKey, hash[:]), nil
}
```

- [ ] **Step 2: 在安装流程中集成签名验证**

```go
func (c *RegistryClient) Install(name, version string) error {
    // 1. 下载插件包
    data := download(name, version)
    // 2. 获取插件元数据（含签名和公钥）
    info := c.Get(name)
    // 3. 验证签名
    if !VerifySignature(data, info.Signature, info.PublicKey) {
        return ErrSignatureInvalid
    }
    // 4. 验证 checksum
    if sha256.Sum256(data) != info.Checksum {
        return ErrChecksumMismatch
    }
    // 5. 安装
    install(data)
}
```

- [ ] **Step 3: 编写测试**

```go
func TestSigning_Verify(t *testing.T) { /* ... */ }
func TestSigning_TamperDetection(t *testing.T) { /* ... */ }
```

---

### Task 4: 插件沙箱隔离

**Files:**
- Create: `internal/tools/plugin_sandbox.go`
- Create: `internal/tools/plugin_sandbox_test.go`

- [ ] **Step 1: 定义插件沙箱接口**

```go
// internal/tools/plugin_sandbox.go
package tools

// PluginSandbox 插件沙箱，限制插件可访问的资源
type PluginSandbox interface {
    // Execute 在沙箱中执行插件
    Execute(ctx context.Context, plugin *Plugin, args map[string]any) (*Result, error)
    // Permissions 返回沙箱权限配置
    Permissions() SandboxPermissions
}

// SandboxPermissions 沙箱权限
type SandboxPermissions struct {
    FileSystemAllowed []string // 允许访问的目录
    NetworkAllowed    bool     // 是否允许网络访问
    MaxMemoryMB       int      // 最大内存
    MaxCPUMs          int      // 最大 CPU 时间
    EnvAllowed        []string // 允许读取的环境变量
}
```

- [ ] **Step 2: 实现基于 scope 的沙箱**

```go
type scopeSandbox struct {
    perm SandboxPermissions
    fs   *security.Sandbox // 复用安全沙箱
}

func (s *scopeSandbox) Execute(ctx context.Context, plugin *Plugin, args map[string]any) (*Result, error) {
    // 1. 检查权限
    // 2. 在受限 context 中执行
    // 3. 超时控制
    // 4. panic recovery
}
```

- [ ] **Step 3: 编写测试**

```go
func TestSandbox_BlocksFileSystemAccess(t *testing.T) { /* ... */ }
func TestSandbox_BlocksNetworkAccess(t *testing.T) { /* ... */ }
func TestSandbox_PanicRecovery(t *testing.T) { /* ... */ }
func TestSandbox_Timeout(t *testing.T) { /* ... */ }
```

---

## Phase 5B：企业版多租户（第 4-6 周）

### Task 5: 租户隔离架构

**Files:**
- Create: `internal/multi Tenant/tenant.go`
- Create: `internal/multi Tenant/tenant_test.go`
- Modify: `internal/memory/memory.go`（支持租户分区）
- Modify: `internal/pool/pool.go`（支持租户配额）
- Modify: `internal/agent/bus.go`（支持租户隔离）

- [ ] **Step 1: 定义租户模型**

```go
// internal/multi Tenant/tenant.go
package multitenant

// Tenant 租户
type Tenant struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Plan        TenantPlan        `json:"plan"`
    Quota       TenantQuota       `json:"quota"`
    CreatedAt   time.Time         `json:"created_at"`
}

// TenantPlan 租户计划
type TenantPlan string

const (
    PlanFree    TenantPlan = "free"
    PlanPro     TenantPlan = "pro"
    PlanEnterprise TenantPlan = "enterprise"
)

// TenantQuota 租户配额
type TenantQuota struct {
    MaxConcurrentAgents int   `json:"max_concurrent_agents"`
    MaxLLMCallsPerDay   int64 `json:"max_llm_calls_per_day"`
    MaxTokensPerDay     int64 `json:"max_tokens_per_day"`
    MaxMemoryMB         int   `json:"max_memory_mb"`
    MaxStorageMB        int   `json:"max_storage_mb"`
}

// TenantContext 租户上下文，通过 context 传播
type TenantContext struct {
    TenantID string
    UserID   string
}
```

- [ ] **Step 2: 实现 Memory Store 租户分区**

```go
// 在 MemoryStore 中增加租户前缀
func tenantKey(tenantID, key string) string {
    return tenantID + ":" + key
}

// InMemoryStore 支持按租户隔离
func (s *InMemoryStore) Add(ctx context.Context, episode *Episode) error {
    tc := TenantFromContext(ctx)
    if tc != nil {
        episode.ID = tenantKey(tc.TenantID, episode.ID)
    }
    // ... 现有逻辑
}
```

- [ ] **Step 3: 实现 Pool 租户配额**

```go
// Pool 按租户限制并发数
func (p *Pool) Dispatch(ctx context.Context, task TaskConfig) error {
    tc := TenantFromContext(ctx)
    if tc != nil {
        active := p.tenantActive.Load(tc.TenantID)
        quota := p.tenantQuotas[tc.TenantID]
        if active >= int64(quota.MaxConcurrentAgents) {
            return ErrTenantQuotaExceeded
        }
    }
    // ... 现有逻辑
}
```

- [ ] **Step 4: 实现配额检查中间件**

```go
// QuotaMiddleware 检查租户配额
func QuotaMiddleware(quotaStore QuotaStore) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tc := TenantFromContext(r.Context())
            if tc != nil {
                if !quotaStore.CheckLLMQuota(tc.TenantID) {
                    http.Error(w, "daily LLM quota exceeded", http.StatusTooManyRequests)
                    return
                }
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

- [ ] **Step 5: 编写测试**

```go
func TestTenantIsolation_Memory(t *testing.T) { /* ... */ }
func TestTenantIsolation_Pool(t *testing.T) { /* ... */ }
func TestTenantQuota_LLMCalls(t *testing.T) { /* ... */ }
func TestTenantQuota_ConcurrentAgents(t *testing.T) { /* ... */ }
```

---

### Task 6: 多租户认证集成

**Files:**
- Create: `internal/multi Tenant/auth.go`
- Create: `internal/multi Tenant/auth_test.go`

- [ ] **Step 1: 实现多模式认证**

```go
// internal/multi Tenant/auth.go

// AuthProvider 认证提供者接口
type AuthProvider interface {
    Authenticate(ctx context.Context, token string) (*TenantContext, error)
}

// APIKeyAuth API Key 认证
type APIKeyAuth struct {
    keys map[string]*TenantContext // apiKey → tenant
}

// OIDCAuth OpenID Connect 认证
type OIDCAuth struct {
    issuer   string
    clientID string
    // ...
}

// SAMLAuth SAML 2.0 认证
type SAMLAuth struct {
    // ...
}
```

- [ ] **Step 2: 编写测试**

```go
func TestAPIKeyAuth(t *testing.T) { /* ... */ }
func TestOIDCAuth(t *testing.T) { /* ... */ }
```

---

## Phase 5C：TypeScript SDK 深化（第 7-9 周）

### Task 7: Edge Runtime 完整支持

**Files:**
- Modify: `sdk/typescript/src/edge/runtime.ts`
- Modify: `sdk/typescript/src/edge/cold-start.ts`
- Create: `sdk/typescript/src/edge/compatibility.ts`

- [ ] **Step 1: 补全 Edge Runtime 兼容层**

```typescript
// sdk/typescript/src/edge/compatibility.ts

// 检测运行时环境
export type RuntimeEnv = 'cloudflare-workers' | 'vercel-edge' | 'deno' | 'node';

export function detectRuntime(): RuntimeEnv {
  if (typeof globalThis.caches !== 'undefined') return 'cloudflare-workers';
  if (typeof globalThis.EdgeRuntime !== 'undefined') return 'vercel-edge';
  if (typeof globalThis.Deno !== 'undefined') return 'deno';
  return 'node';
}

// Edge-compatible fetch（无 Node.js API 依赖）
export function edgeFetch(url: string, init?: RequestInit): Promise<Response> {
  // Cloudflare Workers / Vercel Edge 原生支持 fetch
  return fetch(url, init);
}

// Edge-compatible KV store
export function createEdgeKV(): KVStore {
  const env = detectRuntime();
  switch (env) {
    case 'cloudflare-workers':
      return new CloudflareKVStore(globalThis.env.KV);
    case 'vercel-edge':
      return new VercelEdgeKVStore();
    default:
      return new InMemoryKVStore();
  }
}
```

- [ ] **Step 2: 编写 Edge Runtime 测试**

```typescript
// sdk/typescript/src/edge/__tests__/edge.test.ts
describe('Edge Runtime', () => {
  it('should detect runtime environment', () => { /* ... */ });
  it('should create edge-compatible agent', () => { /* ... */ });
  it('should work without Node.js APIs', () => { /* ... */ });
});
```

---

### Task 8: React Hooks 绑定

**Files:**
- Create: `sdk/typescript/src/react/use-agent.ts`
- Create: `sdk/typescript/src/react/use-react-loop.ts`
- Create: `sdk/typescript/src/react/use-stream-run.ts`
- Create: `sdk/typescript/src/react/index.ts`
- Create: `sdk/typescript/src/react/__tests__/hooks.test.tsx`

- [ ] **Step 1: 实现 `useAgent` Hook**

```typescript
// sdk/typescript/src/react/use-agent.ts
import { useState, useCallback, useRef } from 'react';
import { Agent, AgentConfig } from '../agent/builder';

export interface UseAgentOptions extends Partial<AgentConfig> {
  autoStart?: boolean;
}

export interface UseAgentResult {
  agent: Agent | null;
  isRunning: boolean;
  error: Error | null;
  run: (prompt: string) => Promise<void>;
  streamRun: (prompt: string) => AsyncGenerator<string>;
  stop: () => void;
}

export function useAgent(options: UseAgentOptions = {}): UseAgentResult {
  const [agent, setAgent] = useState<Agent | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const run = useCallback(async (prompt: string) => {
    setIsRunning(true);
    setError(null);
    try {
      const a = new Agent(options).build();
      setAgent(a);
      await a.run(prompt);
    } catch (e) {
      setError(e as Error);
    } finally {
      setIsRunning(false);
    }
  }, [options]);

  const streamRun = useCallback(async function* (prompt: string) {
    const a = new Agent(options).build();
    setAgent(a);
    for await (const chunk of a.streamRun(prompt)) {
      yield chunk;
    }
  }, [options]);

  const stop = useCallback(() => {
    abortRef.current?.abort();
    setIsRunning(false);
  }, []);

  return { agent, isRunning, error, run, streamRun, stop };
}
```

- [ ] **Step 2: 实现 `useReActLoop` Hook**

```typescript
// 实时显示 ReAct Loop 的思考-行动-观察过程
export function useReActLoop(prompt: string) {
  const [thoughts, setThoughts] = useState<string[]>([]);
  const [actions, setActions] = useState<Action[]>([]);
  const [observations, setObservations] = useState<string[]>([]);
  const [currentStep, setCurrentStep] = useState<'thinking' | 'acting' | 'observing' | null>(null);
  // ...
}
```

- [ ] **Step 3: 编写测试**

```typescript
// sdk/typescript/src/react/__tests__/hooks.test.tsx
import { renderHook, act } from '@testing-library/react';
import { useAgent } from '../use-agent';

test('useAgent should run agent', async () => { /* ... */ });
test('useAgent should handle errors', async () => { /* ... */ });
test('useAgent should support streaming', async () => { /* ... */ });
```

- [ ] **Step 4: 在 package.json 中导出 React 子包**

```json
{
  "exports": {
    ".": "./dist/index.js",
    "./react": "./dist/react/index.js"
  }
}
```

---

### Task 9: VS Code 扩展

**Files:**
- Create: `sdk/vscode/package.json`
- Create: `sdk/vscode/src/extension.ts`
- Create: `sdk/vscode/src/debugger.ts`
- Create: `sdk/vscode/src/inspector.ts`

- [ ] **Step 1: 创建 VS Code 扩展骨架**

```typescript
// sdk/vscode/src/extension.ts
import * as vscode from 'vscode';
import { AgentDebugger } from './debugger';
import { AgentInspector } from './inspector';

export function activate(context: vscode.ExtensionContext) {
  // 注册 Agent 调试器
  const debuggerProvider = new AgentDebuggerProvider();
  context.subscriptions.push(
    vscode.debug.registerDebugConfigurationProvider('agentprimordia', debuggerProvider)
  );
  
  // 注册 Agent Inspector Webview
  context.subscriptions.push(
    vscode.commands.registerCommand('agentprimordia.inspect', () => {
      AgentInspector.createOrShow(context);
    })
  );
  
  // 注册 Agent Run 命令
  context.subscriptions.push(
    vscode.commands.registerCommand('agentprimordia.run', async () => {
      const prompt = await vscode.window.showInputBox({ prompt: 'Agent prompt' });
      if (prompt) {
        // 运行 Agent 并在 Webview 中显示结果
      }
    })
  );
}
```

- [ ] **Step 2: 实现 Agent Inspector Webview**

```typescript
// 显示 Agent 状态、ReAct Loop 步骤、工具调用、LLM 响应
// 支持断点、单步执行、检查点恢复
```

---

## Phase 5D：社区运营工具链（第 10-12 周）

### Task 10: 贡献者脚手架

**Files:**
- Create: `ecosystem/contributing/plugin-template/`（插件开发模板）
- Create: `ecosystem/contributing/provider-template/`（Provider 开发模板）
- Modify: `cmd/ap/init.go`（支持 `ap init --type plugin`）

- [ ] **Step 1: 创建插件开发模板**

```
ecosystem/contributing/plugin-template/
├── plugin.json          # 插件清单模板
├── main.go              # 插件入口模板
├── main_test.go         # 测试模板
├── README.md            # 文档模板
├── Makefile             # 构建/测试/发布
└── .github/workflows/   # CI 模板
```

- [ ] **Step 2: `ap init --type plugin` 支持插件项目脚手架**

```go
// cmd/ap/init.go
func newInitCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use: "init",
        Run: func(cmd *cobra.Command, args []string) {
            projectType, _ := cmd.Flags().GetString("type")
            switch projectType {
            case "plugin":
                scaffoldPlugin(opts)
            case "provider":
                scaffoldProvider(opts)
            default:
                scaffoldAgent(opts)
            }
        },
    }
    cmd.Flags().String("type", "agent", "project type: agent, plugin, provider")
    return cmd
}
```

---

### Task 11: 文档站点建设

**Files:**
- Create: `docs/.vitepress/config.ts`（VitePress 配置）
- Create: `docs/guide/`（教程系列）
- Create: `docs/cookbook/`（实战菜谱）
- Create: `docs/api-reference/`（API 参考）

- [ ] **Step 1: 配置 VitePress 站点**

```typescript
// docs/.vitepress/config.ts
import { defineConfig } from 'vitepress';

export default defineConfig({
  title: 'AgentPrimordia',
  description: 'Go 优先的企业级 AI Agent 框架',
  themeConfig: {
    nav: [
      { text: '指南', link: '/guide/' },
      { text: '菜谱', link: '/cookbook/' },
      { text: 'API', link: '/api-reference/' },
      { text: '插件', link: '/plugins/' },
    ],
    sidebar: {
      '/guide/': [
        { text: '快速开始', link: '/guide/getting-started' },
        { text: '核心概念', link: '/guide/concepts' },
        { text: 'ReAct 循环', link: '/guide/react-loop' },
        { text: '工具系统', link: '/guide/tools' },
        { text: '记忆系统', link: '/guide/memory' },
        { text: '多 Agent 编排', link: '/guide/orchestration' },
        { text: '部署运维', link: '/guide/deployment' },
      ],
    },
  },
});
```

- [ ] **Step 2: 编写教程系列**

```
docs/guide/
├── getting-started.md      # 5 分钟快速入门
├── concepts.md              # 核心概念
├── react-loop.md            # ReAct 循环详解
├── tools.md                 # 工具系统
├── memory.md                # 记忆系统
├── orchestration.md         # 多 Agent 编排
├── deployment.md            # 部署运维
├── security.md              # 安全最佳实践
└── performance.md           # 性能调优
```

- [ ] **Step 3: 编写实战菜谱**

```
docs/cookbook/
├── rag-agent.md             # RAG Agent 完整实现
├── multi-agent-collab.md    # 多 Agent 协作
├── code-review-bot.md       # 代码审查 Bot
├── customer-support.md      # 客服 Agent
├── data-analysis.md         # 数据分析 Agent
├── custom-provider.md       # 自定义 LLM Provider
├── custom-tool.md           # 自定义工具
├── k8s-deployment.md        # K8s 部署完整指南
└── plugin-development.md    # 插件开发指南
```

---

### Task 12: 社区 CI 脚手架

**Files:**
- Create: `ecosystem/contributing/plugin-template/.github/workflows/ci.yml`
- Create: `ecosystem/contributing/plugin-template/.github/workflows/release.yml`

- [ ] **Step 1: 插件 CI 模板**

```yaml
# .github/workflows/ci.yml
name: Plugin CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go test -race -coverprofile=coverage.out ./...
      - run: go vet ./...
      - run: gofmt -l .
  
  release:
    needs: test
    if: startsWith(github.ref, 'refs/tags/v')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build plugin
        run: go build -o plugin.so -buildmode=plugin .
      - name: Sign plugin
        run: cosign sign-blob plugin.so
      - name: Publish to registry
        run: ap plugin publish
```

---

## 验收标准

1. `go build ./...` 和 `go vet ./...` 零错误
2. `go test -race -count=1 ./...` 全部通过
3. `ap plugin install/list/search/remove/update` 命令可用
4. 插件下载自动验证 cosign 签名
5. 插件在沙箱中运行，无法越权访问文件系统/网络
6. 多租户 Memory/Pool/Bus 按租户隔离
7. 租户配额限制 LLM 调用次数、并发数、存储
8. TS SDK 支持 Cloudflare Workers / Vercel Edge 运行时
9. React Hooks（`useAgent`/`useReActLoop`）可用且有测试
10. VS Code 扩展可启动 Agent Inspector Webview
11. `ap init --type plugin` 生成插件项目骨架
12. VitePress 文档站点包含 8+ 教程 + 9+ 菜谱
13. 插件 CI 模板可用

## 预期成果

| 指标 | 当前 | 目标 |
|------|------|------|
| 基础插件 | 6 | 6 + 插件市场 |
| CLI 插件命令 | 0 | 5（install/list/search/remove/update） |
| 插件签名验证 | 无 | cosign + checksum |
| 插件沙箱 | 无 | 文件系统/网络/内存隔离 |
| 多租户 | 无 | Memory/Pool/Bus 隔离 + 配额 |
| 认证模式 | API Key | API Key + OIDC + SAML |
| TS SDK 运行时 | Node.js | Node.js + Edge + React |
| React Hooks | 0 | 3（useAgent/useReActLoop/useStreamRun） |
| VS Code 扩展 | 无 | Agent Inspector |
| 文档页面 | ~20 | 50+（教程 + 菜谱 + API 参考） |
| 贡献者模板 | 0 | 2（plugin + provider） |
