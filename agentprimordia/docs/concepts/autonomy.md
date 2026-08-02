# 长期自治（Autonomy）

Autonomy 模块让 Agent 从"用户问 → agent 答"的被动会话，跃迁为"给定目标 → 自主规划、执行、校验、再计划"的主动自治，支持长时间运行与崩溃恢复。对应 V4 路线图 v3.3。

## 核心模型

- **AgentGoal**：持久化目标，含描述、验收标准、优先级、状态机与重试计数。
- **GoalPlan / PlanStep**：目标分解为带依赖（DAG）的有序步骤，支持顺序 / 并行 / 条件策略；内置循环依赖检测与进度计算。
- **GoalExecutor**：按计划逐步执行，步骤失败 → 重试 → 重规划，支持 goroutine 并行步骤与上下文取消。

## 目标状态机

`created → planned → executing → validated → done`；任意非终态可转 `failed`；`validated` 校验不通过可回 `executing` 重规划；`failed` 可回 `planned` 重试。非法转换被状态机拒绝。

## 运行时装配

```go
rt := autonomy.NewAutonomyRuntime(autonomy.RuntimeConfig{
    StepExecutor:    myExecutor,
    CheckpointStore: myStore,    // 每步 checkpoint，崩溃可恢复
    MemoryStore:     myMemory,   // 跨会话记忆 / 失败教训
    ReplanPlanner:   myPlanner,  // 校验不达标自动重规划
    MonitorConfig:   autonomy.MonitorConfig{StallThreshold: 5},
})
goal := rt.SubmitGoal("监控数据异常并修复", autonomy.GoalConfig{Priority: autonomy.PriorityHigh})
_ = rt.SetPlan(goal.ID, plan)
_ = rt.ExecuteGoal(ctx, goal.ID)
```

## 调度与监控

- **Scheduler**：cron 式定时唤醒 + 事件驱动调度（订阅事件总线）。
- **Monitor**：停滞检测（连续 N 轮无进展告警）、步骤级进度追踪、异常分级（warn/error/critical）上报。

## 崩溃恢复与幂等

- **ResumeManager**：启动时 `ResumeIncomplete` 扫描未完成目标，从最后有效 checkpoint 恢复；恢复后校验上下文一致性，不一致则重规划。
- **IdempotencyGuard**：基于 `goalID:stepID:attempt` 的幂等键，防止恢复后重复副作用；`Reset` 按目标精确清除。

## 跨组件集成

自治执行经集成接口对接 RAG（步骤前注入知识）、Pool（多目标并发信号量调度）、集群（跨节点续跑）、可观测（目标级 metrics/trace）、Guardrail（每步护栏校验）。

## 能力注入与公共 API

- 链式注入：`agent.WithAutonomy(ap.AutonomyConfig{Runtime: rt})`，ReAct 引擎经 `AutonomyCapable` 类型断言自动发现。
- 公共 API：`pkg/autonomy.go` 导出 `AgentGoal` / `GoalPlan` / `AutonomyRuntime` / 状态与优先级常量。
- CLI：`ap autonomy run|list|resume|status`。

## 相关文档

- 验收 demo：`ecosystem/examples/autonomous-task/`
- 路线图：`docs/V4-ROADMAP.md` §二
