# 增强规划示例（Planning Enhanced）

本示例演示 AgentPrimordia v7.1 增强规划能力的完整工作流。

## 展示内容

| 组件 | 类型 | 说明 |
|------|------|------|
| EnhancedPlanner | `planning.EnhancedPlanner` | 组合 Base+Replanner+Recovery+Deadlock+Approval 的统一规划器 |
| ManagedPlan | `planning.ManagedPlan` | 计划状态机：pending → active → blocked → completed/failed |
| LLMReplanner | `planning.LLMReplanner` | 执行偏离时由 LLM 驱动的动态重规划 |
| DeadlockDetector | `planning.DeadlockDetector` | 连续失败超过阈值即判定死路，触发恢复策略 |
| PolicyApprovalGate | `planning.PolicyApprovalGate` | 高风险动作（如 deploy）需审批后方可执行 |

## 运行

```bash
cd agentprimordia/
go run ./ecosystem/examples/planning-enhanced/
```

## 示例流程

1. **生成计划** — 使用 EnhancedPlanner 将复杂任务分解为子任务
2. **状态机管理** — 将计划包装为 ManagedPlan，驱动合法状态转换
3. **执行 + 重规划** — 模拟子任务失败后，LLMReplanner 判断是否需要重规划
4. **死路检测** — DeadlockDetector 检测连续失败，触发 LLMRecoveryStrategy 生成替代方案
5. **审批门** — PolicyApprovalGate 对高风险动作（deploy）要求审批后才放行
6. **状态转换历史** — 打印完整的状态机变迁记录

## 关键点

- 使用 `testutil.MockProvider` 预设响应序列，无需真实 LLM API
- 所有 LLM 调用（Decompose / ShouldReplan / Replan / Recover）通过 Mock 模拟
- 审批门使用 goroutine + channel 实现异步审批等待
- 状态转换遵循合法路径约束，非法转换会返回错误
