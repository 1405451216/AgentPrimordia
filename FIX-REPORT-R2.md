# AgentPrimordia 第二轮修复报告

**修复时间**: 2026-06-10  
**修复范围**: 剩余 ~15 个问题（评估报告中的 P2/P3 级别）  
**测试结果**: 20/20 包通过，0 失败

---

## 修复清单

### 1. DAG 条件边 remainingDeps 传播 (internal/agent/dag.go)

**问题**: 节点执行 goroutine 中，跳过路径和执行路径分别维护 `result.Order` 追加和 `remainingDeps` 递减，代码冗余且条件边评估不安全（跳过的源节点仍参与条件函数调用）。

**修复**:
- 使用 `defer` 统一处理 Order 追加和 remainingDeps 递减，确保无论成功/失败/跳过/panic 都正确传播
- 条件边评估增加 `!srcResult.Skipped && srcResult.Error == nil` 保护，仅对实际执行成功的源节点评估条件函数
- 增加 `remainingDeps[dst] > 0` 防护，避免计数器下溢

```go
defer func() {
    stateMu.Lock()
    result.Order = append(result.Order, nid)
    for _, edgeIdx := range outgoing[nid] {
        dst := edges[edgeIdx].To
        if remainingDeps[dst] > 0 {
            remainingDeps[dst]--
        }
    }
    stateMu.Unlock()
}()
```

---

### 2. Workflow 状态机无限循环保护 (internal/agent/workflow.go)

**问题**: `executeStateMachine` 没有迭代次数限制，条件配置错误时可能死循环。

**修复**: 添加 `maxIter` 计数器，默认使用 `config.MaxIterations` 或 `defaultMaxIterations (100)`，超限时返回明确错误。

```go
iterations++
if iterations > maxIter {
    return fmt.Errorf("state machine exceeded max iterations (%d), possible infinite loop at state %q", maxIter, currentState)
}
```

---

### 3. Workflow currentNode 数据竞争 (internal/agent/workflow.go)

**问题**: `w.currentNode = node` 在 `executeNode` 中无锁写入，但其他方法（如 GetStatus）通过 RLock 读取。

**修复**: 写入时获取 `w.mu.Lock()`。

---

### 4. Workflow Pause/Resume 设计缺陷 (internal/agent/workflow.go)

**问题**: `Pause()` 调用 `cancelFunc()` 取消 context，导致执行循环退出。`Resume()` 虽然创建新 context，但执行循环已终止，无法恢复。

**修复**:
- `Pause()` 不再取消 context，改为向 `pauseCh` 发送信号
- 执行循环在每次节点执行后调用 `checkPause(ctx)`，检查暂停状态并阻塞等待恢复
- `Resume()` 关闭 `pauseCh` 解除阻塞，然后重建新 `pauseCh` 以支持后续暂停
- 同时为 `executeLinear` 添加暂停检查

```go
func (w *WorkflowExecution) checkPause(ctx context.Context) error {
    w.mu.RLock()
    if w.status != WfStatusPaused {
        w.mu.RUnlock()
        return nil
    }
    pauseCh := w.pauseCh
    w.mu.RUnlock()
    select {
    case <-pauseCh:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

---

### 5. Broadcast 同步阻塞 (internal/agent/bus.go)

**问题**: `Broadcast` 在持有 RLock 期间同步调用所有 handler，长时间持有读锁阻塞写操作。

**修复**:
- 在 RLock 下快照 handlers 和 channels
- 释放锁后异步调用 handler（goroutine 池）
- 使用 `sync.WaitGroup` + `sync.Mutex` 收集结果

---

### 6. CostTracker 预算检查 TOCTOU (internal/agent/cost_tracker.go)

**问题**: `Record()` 在 `Lock/Unlock` 后调用 `CheckBudget()`（再次 RLock），中间存在竞态窗口。`MaxTokensPerCall` 从未被检查。

**修复**:
- 新增 `checkBudgetLocked()` 方法在持有锁时检查预算
- `Record()` 在同一次 Lock 中完成记录 + 预算检查
- `CheckBudget()` 改为委托到 `checkBudgetLocked()`
- 增加 `MaxTokensPerCall` 单次调用上限检查

---

### 7. HookStats 原子/互斥混合 (internal/agent/hooks.go)

**问题**: `Record()` 使用 `atomic.AddInt64` 写 TotalFired/TotalErrors，但 `Snapshot()` 仅用 RLock 读取（非原子），构成数据竞争。

**修复**: `Snapshot()` 改用 `atomic.LoadInt64` 读取 TotalFired 和 TotalErrors。

---

### 8. A2A 事件通道泄漏 (internal/agent/a2a/task_manager.go)

**问题**: `Unsubscribe` 不关闭通道，消费者 goroutine 泄漏。`Cleanup` 同样不关闭通道。`publishEventLocked` 使用 `default` 立即丢弃事件。

**修复**:
- `Unsubscribe`: 删除通道后立即 `close(ch)`
- `Cleanup`: 遍历所有订阅者通道并关闭
- `publishEventLocked`: `default` 替换为 `time.After(100ms)` 超时，避免慢消费者阻塞发布者

---

### 9. MCP stdio 管道管理 (internal/tools/mcp_registry.go)

**问题**: stdin/stdout 管道创建后被 `_ = stdin; _ = stdout` 丢弃，子进程无法通过 stdio 通信。

**修复**:
- `MCPClientEntry` 新增 `Stdin io.WriteCloser` 和 `Stdout io.ReadCloser` 字段
- `startProcess` 保存管道到 entry
- `Stop` 方法增加管道关闭清理

---

### 10. Sandbox/Shell 安全统一 (internal/tools/builtin/shell.go)

**问题**: Shell 工具有自己的安全模型（白名单/黑名单/元字符过滤），但不通过 Sandbox 安全检查，两套安全层并行运行。

**修复**:
- 新增 `SandboxChecker` 接口（`CanExecute` + `ValidatePath`）
- Shell 增加 `WithSandbox(sandbox, agentID)` 注入方法
- Execute 方法在白名单/黑名单检查后，若注入 Sandbox 则追加 `CanExecute` 和 `ValidatePath` 验证

---

### 11. Metrics 标签维度 (internal/metrics/metrics.go)

**问题**: `RecordLLMCall` 和 `RecordToolCall` 仅记录标量计数器，不支持 provider/model/agent_name 标签维度，但 Grafana Dashboard 期望带标签的指标。

**修复**:
- 新增 `RecordLLMCallWithLabels(duration, err, provider, model)` 方法
- 新增 `RecordToolCallWithLabels(duration, err, toolName)` 方法
- 新增 `RecordTurnWithAgent(duration, agentName)` 方法
- 新增 `LLMCallsByLabel`/`ToolCallsByLabel`/`TurnsByAgent` map 追踪
- `String()` 输出增加 `ap_llm_calls_by_provider{provider,model}`、`ap_tool_calls{tool_name}`、`ap_turns{agent_name}` 等带标签指标
- `Reset()` 同步清理标签计数器

---

### 12. Grafana Dashboard PromQL 修复 (deploy/grafana/)

**问题**: Dashboard JSON 中的 PromQL 查询引用不存在的指标名称和标签。

**修复**:
- `dashboard-agent.json`: `ap_tool_total_calls_total` → `ap_tool_calls`，错误率公式改用 `ap_llm_total_errors / ap_llm_total_calls`，模板变量改用 `ap_llm_calls_by_provider`
- `dashboard-cost.json`: `ap_cost_total_dollars` → `ap_cost_usd` 占位符，增加说明面板
- `dashboard-llm.json`: `ap_llm_total_calls{status="error"}` → `ap_llm_total_errors`，饼图改用 `ap_llm_calls_by_provider`

---

### 13. Operator healthCheck 注入 (operator/controller/agent_controller.go)

**问题**: CRD 和 Go 类型定义了 `HealthCheck` 探针，但 `ensureDeployment` 未将其注入 Pod 容器规格。

**修复**: 在 `ensureDeployment` 中检测 `agentDeploy.Spec.HealthCheck`，非 nil 时构建 `corev1.Probe` 并设置到 agent 容器的 `LivenessProbe` 和 `ReadinessProbe`。

---

### 14. Operator Status 字段填充 (operator/controller/agent_controller.go)

**问题**: `AverageTurnLatencySeconds`、`TotalTokens`、`EstimatedCostUSD` 在 Go 类型中声明但从未被 controller 赋值。

**修复**: 在 `updateStatus` 中添加占位赋值（=0），附带 TODO 注释说明生产环境应从 metrics sidecar 抓取。

---

### 15. CRD Schema 同步 (operator/manifest/crd.yaml)

**问题**: Go 类型中的 `metrics`、`tracing`、`averageTurnLatencySeconds` 等字段未出现在 CRD YAML schema 中。

**修复**:
- 添加 `template.metrics` 对象（enabled, path, port, serviceMonitor）
- 添加 `template.tracing` 对象（enabled, otlpEndpoint, samplingRate）
- 添加 status 字段（averageTurnLatencySeconds, totalTokens, estimatedCostUSD）
- 添加 healthCheck 探针的 successThreshold/failureThreshold 字段

---

## 测试结果

```
ok  agentprimordia/internal/admin        0.285s
ok  agentprimordia/internal/agent        24.427s
ok  agentprimordia/internal/agent/a2a    1.343s
ok  agentprimordia/internal/concurrency  0.127s
ok  agentprimordia/internal/config       1.630s
ok  agentprimordia/internal/debugger     0.082s
ok  agentprimordia/internal/events       0.171s
ok  agentprimordia/internal/guardrail    0.047s
ok  agentprimordia/internal/llm          47.506s
ok  agentprimordia/internal/memory       0.881s
ok  agentprimordia/internal/metrics      0.318s
ok  agentprimordia/internal/orchestration 0.233s
ok  agentprimordia/internal/otel         2.078s
ok  agentprimordia/internal/persist      0.082s
ok  agentprimordia/internal/pool         9.040s
ok  agentprimordia/internal/prompt       0.034s
ok  agentprimordia/internal/security     0.034s
ok  agentprimordia/internal/tools        0.542s
ok  agentprimordia/internal/tools/builtin 12.477s
ok  agentprimordia/pkg                   0.039s
```

**20/20 包通过 | 0 失败**

---

## 两轮修复累计统计

| 轮次 | 修复数 | 通过包 | 失败包 |
|------|--------|--------|--------|
| 第一轮 | 17 | 20 | 0 |
| 第二轮 | 15 | 20 | 0 |
| **总计** | **32** | **20** | **0** |
