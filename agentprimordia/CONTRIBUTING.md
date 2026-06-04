# 🤝 贡献指南

感谢你对 AgentPrimordia 的关注！我们欢迎所有形式的贡献，包括但不限于：

- 🐛 Bug 修复
- ✨ 新功能
- 📝 文档改进
- 🧪 测试用例
- 💡 建议/想法
- 🌍 国际化翻译

## 📋 贡献流程

### 1. 准备工作

```bash
# Fork 仓库到你的 GitHub 账户
# 克隆你的 Fork
git clone https://github.com/YOUR_USERNAME/agentprimordia.git
cd agentprimordia

# 添加上游仓库（可选）
git remote add upstream https://github.com/original/agentprimordia.git

# 创建功能分支
git checkout -b feature/your-feature-name
```

### 2. 开发规范

#### 代码风格

- **语言**: Go 1.22+
- **格式化**: 使用 `gofmt` 或 `go fmt`
- **注释**: 中文注释（代码注释使用中文）
- **命名**: 遵循 Go 官方命名规范

```go
// ✅ 好的命名和注释
type MemoryStore struct {
    db *sql.DB // 数据库连接
}

// Add 添加一条新的记忆记录
func (s *MemoryStore) Add(ctx context.Context, episode *Episode) error {
    // 实现...
}
```

#### TDD 开发模式

**强制要求：先写测试！**

```go
// 步骤1: 先写测试（Red）
func TestNewFeature(t *testing.T) {
    result := NewFeature()
    if result == nil {
        t.Error("expected non-nil result")
    }
}

// 步骤2: 实现功能（Green）
func NewFeature() *Feature {
    return &Feature{}
}

// 步骤3: 重构优化（Refactor）
```

#### 模块边界

> **2026-06 更新**: Phase 6 起 `agent/` 实际处于依赖顶层（依赖 llm/memory/persist/tools），
> 旧的"不依赖 pool/memory"描述已不准。详见 `docs/plans/2026-06-04-phase6-implementation.md` §模块边界更新。

```
internal/
├── agent/      — ReActLoop 引擎 + 协议式微内核（顶层，依赖 llm/memory/persist/tools）
├── pool/       — 多 Agent 调度（依赖 agent, tools）
├── tools/      — 工具系统（独立模块，被 agent/pool 依赖）
├── memory/     — 记忆存储（独立模块，被 agent 依赖）
├── llm/        — LLM 抽象层（最底层，被 agent 依赖）
└── persist/    — 状态持久化（独立模块，被 agent 依赖）
```

实际依赖图：

```
        ┌────────────────────────────────────────┐
        │           agent/  (顶层)               │
        │   引用 llm, memory, persist, tools    │
        └────┬───────┬───────┬───────────┬──────┘
             │       │       │           │
        ┌────▼─┐ ┌───▼──┐ ┌──▼───┐ ┌────▼────┐
        │ llm  │ │memory│ │persist│ │  tools  │
        └──────┘ └──────┘ └───────┘ └────┬────┘
                                          │
                                     ┌────▼────┐
                                     │  pool   │
                                     └─────────┘
```

- `internal/*` 之间：`agent/` 处于顶层，可引用下层；下层（llm/memory/persist/tools）不能反向引用 `agent/`
- `pkg/` 只做类型导出和 re-export，不含业务逻辑
- `ecosystem/` 与 `internal/` 互不依赖：`ecosystem/plugins/*` 等通过 `tools.Plugin` 协议与核心解耦

#### 流程约束（2026-06 强化）

任何新 Phase 必须先有 `docs/plans/YYYY-MM-DD-phaseN-implementation.md` 才有 commit。Phase 6 是反例（代码先行、文档后补），后续严禁重蹈。

`// Deprecated:` 标注必须包含 `// Removed in vX.Y.`。

#### 提交信息规范

使用 Conventional Commits 格式：

```
feat: 添加新的 LLM Provider 支持
fix: 修复 SQLite 并发写入问题
refactor: 重构工具注册机制
docs: 更新 API 文档
test: 添加 Memory 模块集成测试
chore: 升级依赖版本
```

### 3. 测试要求

#### 单元测试

每个新功能必须有对应测试：

```go
func TestYourFunction_EdgeCases(t *testing.T) {
    tests := []struct {
        name     string
        input    InputType
        expected ExpectedType
        wantErr  bool
    }{
        {"正常情况", normalInput, normalResult, false},
        {"边界条件", edgeInput, edgeResult, false},
        {"错误输入", badInput, nil, true},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            result, err := YourFunction(tc.input)
            if (err != nil) != tc.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tc.wantErr)
            }
            if !reflect.DeepEqual(result, tc.expected) {
                t.Errorf("result = %v, expected %v", result, tc.expected)
            }
        })
    }
}
```

#### 使用临时目录

文件相关测试必须使用 `t.TempDir()`：

```go
func TestFileOperation(t *testing.T) {
    tmpDir := t.TempDir() // 自动清理
    filePath := filepath.Join(tmpDir, "test.txt")

    err := WriteFile(filePath, "content")
    if err != nil {
        t.Fatalf("WriteFile failed: %v", err)
    }

    // 测试完成后，tmpDir 会自动删除
}
```

#### Mock LLM 用于 Agent/Pool 测试

```go
func TestAgentWithMockLLM(t *testing.T) {
    mockLLM := llm.NewMockLLM()
    mockLLM.SetResponse("测试响应")
    mockLLM.SetError(nil)

    agent := agent.NewReActAgent(agent.ReActConfig{
        Name:  "TestAgent",
        Model: mockLLM,
    })

    resp, err := agent.Run(context.Background(), agent.UserMessage("测试"))
    // 断言...
}
```

### 4. PR 流程

#### 提交 PR 前

- [ ] 代码通过 `go build ./...` 编译
- [ ] 所有测试通过 `go test ./... -v`
- [ ] 代码格式化 `go fmt ./...`
- [ ] 无 lint 错误 `golangci-lint run`（如果有配置）
- [ ] 新功能有对应测试
- [ ] 文档已更新（如果涉及 API 变更）
- [ ] Commit message 符合规范

#### PR 模板

```markdown
## 📝 变更描述
简要描述这个 PR 做了什么改动

## 🔗 相关 Issue
Fixes #123

## 📸 截图/演示（如适用）
[添加截图或 GIF]

## ✅ 变更类型
- [ ] Bug 修复
- [ ] 新功能
- [ ] 破坏性变更
- [ ] 文档更新

## 🧪 测试说明
描述如何测试这些变更

## 📚 补充说明
其他需要审查者注意的信息
```

## 🎯 贡献方向

### 高优先级（欢迎贡献）

1. **新 LLM Provider**
   - Claude 3.5 Sonnet
   - Mistral Large
   - 本地模型支持（llama.cpp）
   - 更多国内模型（文心一言、讯飞星火等）

2. **内置工具扩展**
   - 数据库操作工具（MySQL、PostgreSQL）
   - Git 操作工具
   - Docker/K8s 管理工具
   - Email 发送工具

3. **文档和示例**
   - 视频教程
   - 博客文章
   - 最佳实践案例
   - 多语言文档

4. **性能优化**
   - 连接池优化
   - 缓存策略
   - 批量操作优化

### 中等优先级

5. **UI/可视化**
   - Web Dashboard
   - CLI 工具增强
   - Grafana Dashboard 模板

6. **集成生态**
   - LangChain 兼容层
   - OpenAI Plugins 适配器
   - MCP (Model Context Protocol) 支持

7. **安全增强**
   - RBAC 权限系统
   - 审计日志
   - 敏感数据加密

## 💡 贡献建议

### 初学者友好任务

标记为 `good first issue` 的任务适合首次贡献者：
- 文档错别字修复
- 示例代码改进
- 简单的 Bug 修复
- 测试覆盖率提升

### 如何选择任务

1. 查看 [Issues](https://github.com/your-org/agentprimordia/issues)
2. 筛选标签：`good first issue`, `help wanted`, `documentation`
3. 评论表示你要处理该 Issue
4. 等待 Maintainer 分配后开始工作

## 🏆 贡献者认可

### 贡献者列表

所有贡献者都会被添加到 [CONTRIBUTORS.md](./CONTRIBUTORS.md)。

### 贡献等级

| 等级 | 要求 | 徽章 |
|------|------|------|
| 🥉 Bronze | 1+ 合并 PR | Contributor |
| 🥈 Silver | 5+ 合并 PR | Active Contributor |
| 🥇 Gold | 10+ 合并 PR + Reviewer | Core Contributor |
| 💎 Diamond | 20+ PR + 核心维护 | Maintainer |

## 📞 联系方式

- **Discord**: [加入我们的 Discord](https://discord.gg/xxxxx)
- **微信群**: 扫码加入（在 README 中找二维码）
- **Email**: contributors@agentprimordia.dev
- **Issue**: 在 GitHub 提交 Issue

## ⚖️ 行为准则

我们的社区遵循 **Contributor Covenant** 行为准则：

- ✅ 尊重他人
- ✅ 接受建设性批评
- ✅ 关注对社区最有利的事情
- ✅ 对其他社区成员展现同理心

❌ 不容忍的行为：
- 性别歧视、性化语言
- 人身攻击或政治攻击
- 公开或私下的骚扰
- 未经许可发布他人的私人信息

## ❓ 常见问题

### Q: 我可以提交大型重构吗？
A: 可以，但建议先提 Issue 讨论，获得维护者同意后再开始。

### Q: 我的 PR 多久会被 review？
A: 通常 1-3 个工作日。如果是紧急修复会更快速。

### Q: 可以同时提交多个 PR 吗？
A: 可以，但建议每个 PR 只做一件事，便于 review 和合并。

### Q: 如何成为 Maintainer？
A: 持续高质量贡献，参与代码 review，帮助社区成员。当达到一定活跃度后，现有 Maintainer 会邀请你加入。

---

**🙏 再次感谢你的贡献！让我们一起打造最好的 AI Agent 框架！**

💖 **Star 这个项目** 如果你觉得它有用的话！
