# Go ↔ TypeScript 功能差异分析

## 已对齐模块

| 模块 | Go 导出数 | TS 导出数 | 覆盖率 | 备注 |
|------|-----------|-----------|--------|------|
| Agent (ReAct) | ~120 | ~45 | 38% | TS 聚焦核心循环，高级特性委托 Go |
| LLM Provider | ~80 | ~35 | 44% | 多 Provider 支持已对齐 |
| Memory | ~60 | ~30 | 50% | InMemory + Vector 已对齐 |
| Tools | ~90 | ~25 | 28% | TS 仅内置工具，复杂工具走 Go |
| Chaos | ~50 | ~30 | 60% | v3.0 新增 |
| Cluster | ~40 | ~20 | 50% | v3.0 新增，轻量实现 |
| Errors | 36 | 36 | 100% | 完全对齐 |

## 未移植模块（设计决策）

### governance/ — 治理引擎
- **原因**: 治理策略（审计日志、合规检查、访问控制）强依赖 Go 后端的持久化和分布式一致性
- **替代方案**: TS 通过 A2A 协议调用 Go 治理服务

### registry/ — Agent 市场
- **原因**: 市场协议涉及签名验证、模板管理、版本控制等重逻辑
- **替代方案**: TS 提供市场客户端 API（HTTP），核心逻辑在 Go 端

### persist/ — 分布式检查点
- **原因**: etcd/Redis 后端无 TS 等价实现
- **替代方案**: TS 使用 InMemoryCheckpointStore（单机）或委托 Go 后端

### security/ — 沙箱/ACL
- **原因**: 路径校验、命令过滤等安全逻辑需要 OS 级访问
- **替代方案**: TS 在 Edge 环境使用 WASM 沙箱（src/edge/sandbox.ts）

## 性能差异说明

| 场景 | Go | TypeScript | 原因 |
|------|-----|-----------|------|
| 向量搜索 (10K) | 2.18ms | 0.93ms (500) | TS 数据集较小；Go 含 HNSW 索引开销 |
| Memory 搜索 (1K) | 0.029ms | 0.08ms (1K) | Go 使用 SQLite FTS；TS 纯内存 |
| JSON 序列化 | — | 640K ops/s | TS V8 原生优化 |
| Agent 单轮 | ~1ms | ~2ms | Go 无 IPC 开销 |
