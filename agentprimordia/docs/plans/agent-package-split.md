# Agent 包拆分计划

> **状态：已完成** ✅
> **完成日期：2026-06-22**

## 目标
将 `internal/agent/` 包按功能拆分为独立子包，降低耦合度，提升可维护性。

## 拆分原则
1. **依赖方向**：子包不能反向依赖 `agent/`，只能依赖 `agent/` 定义的接口
2. **API 稳定性**：保持对外 API 不变，通过 `pkg/` 层重新导出
3. **渐进式拆分**：从最独立的模块开始，逐步推进
4. **测试覆盖**：每个子包拆分后必须通过编译和测试

## 子包结构

### 1. transport/ - 传输层
**文件**：
- `transport.go` - 传输接口定义
- `http_transport.go` - HTTP 传输实现
- `tcp_transport.go` - TCP 传输实现

**依赖**：
- 依赖 `agent/` 的 `Agent` 接口
- 依赖 `internal/llm` 的类型

**导出**：
- `Transport` 接口
- `HTTPTransport` 结构
- `TCPTransport` 结构

### 2. discovery/ - 服务发现
**文件**：
- `discovery.go` - 服务发现实现

**依赖**：
- 依赖 `transport/` 子包
- 依赖 `agent/` 的 `Agent` 接口

**导出**：
- `Discovery` 接口
- `LocalDiscovery` 实现

### 3. bus/ - 消息总线
**文件**：
- `bus.go` - 消息总线实现

**依赖**：
- 无外部依赖

**导出**：
- `MessageBus` 接口
- `LocalMessageBus` 实现

### 4. lifecycle/ - 生命周期管理
**文件**：
- `lifecycle.go` - 生命周期状态机

**依赖**：
- 无外部依赖

**导出**：
- `Lifecycle` 结构
- 状态常量

### 5. session/ - 会话管理
**文件**：
- `session.go` - 会话状态管理

**依赖**：
- 依赖 `agent/` 的 `Message` 类型

**导出**：
- `Session` 结构

### 6. orchestration/ - 编排模块
**文件**：
- `orchestration.go` - 编排接口
- `dag.go` - DAG 编排
- `dag_builder.go` - DAG 构建器
- `dag_delegate.go` - DAG 委托
- `workflow.go` - 工作流

**依赖**：
- 依赖 `agent/` 的 `Agent` 接口
- 依赖 `bus/` 子包

**导出**：
- `Orchestrator` 接口
- `DAGOrchestrator` 实现
- `Workflow` 结构

### 7. collaboration/ - 协作模块
**文件**：
- `collaboration.go` - 协作逻辑
- `group_chat.go` - 群聊实现

**依赖**：
- 依赖 `agent/` 的 `Agent` 接口
- 依赖 `bus/` 子包

**导出**：
- `GroupChat` 结构

### 8. multimodal/ - 多模态处理
**文件**：
- `multimodal.go` - 多模态类型定义
- `multimodal_adapter.go` - 适配器

**依赖**：
- 无外部依赖

**导出**：
- `ContentPart` 类型
- 多模态处理函数

### 9. trace/ - 追踪模块
**文件**：
- `trace.go` - 追踪数据结构
- `tracer.go` - Tracer 实现

**依赖**：
- 无外部依赖

**导出**：
- `Tracer` 接口
- `Trace` 结构

### 10. eval/ - 评估模块
**文件**：
- `eval.go` - 评估逻辑

**依赖**：
- 依赖 `agent/` 的 `Response` 类型

**导出**：
- `Evaluator` 接口

### 11. visualize/ - 可视化模块
**文件**：
- `visualize.go` - 可视化实现

**依赖**：
- 依赖 `agent/` 的类型

**导出**：
- 可视化函数

## 实施顺序
1. **Phase 1**：拆分独立模块（bus, lifecycle, multimodal, trace）
2. **Phase 2**：拆分依赖较少的模块（session, transport, discovery）
3. **Phase 3**：拆分复杂模块（orchestration, collaboration, eval, visualize）
4. **Phase 4**：更新 `pkg/` 导出，验证所有测试通过

## 风险与缓解
1. **循环依赖**：通过接口定义在 `agent/` 层，实现在子包层来避免
2. **API 破坏**：通过 `pkg/` 层重新导出，保持对外 API 不变
3. **测试失败**：每个子包拆分后立即运行测试验证
