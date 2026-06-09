# AgentPrimordia 修复记录

> 修复日期: 2026-06-10 | 涉及文件: 10 个 | 修复问题: 17 个 | 测试状态: 全部通过 (20/20 包)

---

## P0 — Critical 修复

### C1: Panic Recovery 丢失错误
**文件:** `internal/agent/react_loop.go`
**问题:** `reactLoopEngine` panic 后返回 `(nil, nil)`，调用方无法感知崩溃
**修复:** 改用命名返回值 `(resp *Response, err error)`，在 recover 块中赋值

### C2: Pool 重试信号量死锁
**文件:** `internal/pool/dispatcher.go`
**问题:** `executeTask` 中 semaphore 在循环内获取但 defer 在函数返回时才释放，`continue` 重试时双重获取导致死锁
**修复:** 将 semaphore 获取移到循环外部，重试期间持有不释放

### VULN-1: nil ACL 默认允许所有访问
**文件:** `internal/security/sandbox.go`
**问题:** `CanAccess` 中 nil ACL 返回 nil（允许），违反最小权限原则
**修复:** nil ACL 默认返回 ErrAccessDenied（拒绝）

### VULN-2: SQL 注入风险
**文件:** `internal/tools/data_tools.go`
**问题:** `PRAGMA table_info(%s)` 直接拼接表名，未验证
**修复:** 添加 `isValidTableName` 函数，仅允许字母/数字/下划线；使用 `%q` 格式化

### Pool.Close 双重调用 panic
**文件:** `internal/pool/dispatcher.go`
**问题:** `Close()` 无 `sync.Once` 保护，双重调用 panic（close of closed channel）
**修复:** 添加 `closeOnce sync.Once` 字段包裹 close 操作

---

## P1 — High 修复

### H1: fireHook 使用 context.Background()
**文件:** `internal/agent/react_loop.go`
**问题:** Hook 使用 `context.Background()` 脱离 agent 生命周期，且始终返回 nil 导致验证钩子失效
**修复:** 添加 `hookCtx` 字段绑定到运行 context；传播 hook 错误而非忽略

### H2/H3: Lifecycle Pause/Resume TOCTOU 竞态
**文件:** `internal/agent/lifecycle.go`
**问题:** `Pause()`/`Resume()` check-then-act 分离（RLock 检查 → Unlock → 再 Lock 操作）
**修复:** 全部改为单 Lock 原子操作，在锁内复制 listeners/hooks 后在锁外通知

### H3: Lifecycle Reset 数据竞态
**文件:** `internal/agent/lifecycle.go`
**问题:** Unlock 后访问 `l.status` 和 `l.hooks`，其他 goroutine 可能已修改
**修复:** 在锁内复制 listeners 和 hooks 切片，锁外使用副本通知

### H4: GroupChat 独占锁
**文件:** `internal/agent/group_chat.go`
**问题:** `Run()` 持有互斥锁覆盖整个长时间运行
**修复:** 释放锁后运行循环，每轮添加 `ctx.Done()` 检查

### H5: RoleBasedSelector 始终选第一个
**文件:** `internal/agent/group_chat.go`
**问题:** 每次创建新 RoundRobinSelector，index 始终为 0
**修复:** 使用 `len(messages) % len(agents)` 作为轮询索引

---

## P2 — Medium 修复

### M1: callToolsWithRetry / completeWithRetry 无重试
**文件:** `internal/agent/react_loop.go`
**问题:** 方法名含 "WithRetry" 但仅单次调用
**修复:** 添加 maxRetries=2 的重试循环，指数退避，尊重 context 取消

### M2: 异步 summary goroutine 泄漏
**文件:** `internal/agent/react_loop.go`
**问题:** 使用 `context.Background()` 脱离 agent 生命周期，goroutine 可能无限积累
**修复:** 绑定到 `a.hookCtx`，agent 取消时 summary 也取消

### M7: Pipeline/Handoff/Parallel 未检查 ctx.Done()
**文件:** `internal/agent/orchestration.go`
**问题:** 编排循环不检查 context 取消，继续调度工作
**修复:** 在 Pipeline/Handoff 循环开头和 ParallelRun 启动前添加 `ctx.Err()` 检查

### M9: ResilientProvider Stream 无重试
**文件:** `internal/llm/resilient.go`
**问题:** Stream 主 Provider 无重试，瞬时错误直接降级
**修复:** 主 Provider 添加 1 次重试

### 退避缺少 Jitter
**文件:** `internal/llm/resilient.go`
**问题:** 指数退避无随机抖动，多并发请求可能同时重试（惊群效应）
**修复:** 添加 `rand.Float64() * RetryBackoff * 0.5` 抖动，结果不超过 MaxBackoff

### Embeddings 无熔断保护
**文件:** `internal/llm/resilient.go`
**问题:** Embeddings 直接调用主 Provider，不经过熔断器
**修复:** 添加 `recordSuccess()`/`recordFailure()` 调用

### ResumeFromCheckpoint Duration 重置
**文件:** `internal/agent/react_loop.go`
**问题:** 从 checkpoint 恢复时 `startTime = time.Now()`，丢失已运行时长
**修复:** 解析 `state.Metrics.Duration` 并回推 startTime

---

## 修复涉及的文件清单

| 文件 | 修改数 |
|------|:------:|
| `internal/agent/react_loop.go` | 6 |
| `internal/pool/dispatcher.go` | 3 |
| `internal/agent/lifecycle.go` | 3 |
| `internal/agent/group_chat.go` | 2 |
| `internal/llm/resilient.go` | 4 |
| `internal/agent/orchestration.go` | 3 |
| `internal/security/sandbox.go` | 1 |
| `internal/security/sandbox_test.go` | 2 |
| `internal/tools/data_tools.go` | 1 |

## 测试验证

```
ok  agentprimordia/internal/admin          0.257s
ok  agentprimordia/internal/agent          24.397s
ok  agentprimordia/internal/agent/a2a      1.313s
ok  agentprimordia/internal/concurrency    0.131s
ok  agentprimordia/internal/config         1.632s
ok  agentprimordia/internal/debugger       0.055s
ok  agentprimordia/internal/events         0.152s
ok  agentprimordia/internal/guardrail      0.028s
ok  agentprimordia/internal/llm            48.220s
ok  agentprimordia/internal/memory         0.823s
ok  agentprimordia/internal/metrics        0.281s
ok  agentprimordia/internal/orchestration  0.199s
ok  agentprimordia/internal/otel           2.055s
ok  agentprimordia/internal/persist        0.049s
ok  agentprimordia/internal/pool           9.013s
ok  agentprimordia/internal/prompt         0.025s
ok  agentprimordia/internal/security       0.024s
ok  agentprimordia/internal/tools          0.479s
ok  agentprimordia/internal/tools/builtin  12.420s
ok  agentprimordia/pkg                     0.043s
```

**20/20 包全部通过，0 失败。**
