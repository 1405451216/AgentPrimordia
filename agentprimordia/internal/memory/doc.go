// Package memory 提供 AgentPrimordia 的记忆存储系统。
//
// # 内部架构（五大关注点）
//
// 本包含 61 个源文件，按职责可分为以下五个逻辑层：
//
// ## 1. Episode 存储（CRUD）
//
// 核心接口 [Memory] 定义了 Episode 的增删改查操作。
// 实现：[InMemoryStore]（开发/测试）、SQLite（生产，见 sqlite.go）。
//
// ## 2. RAG 管道（检索增强生成）
//
// 文件：rag.go, rag_pipeline.go, rag_generator.go, rag_rerank.go, rag_fusion.go
// 流程：文档加载 → 文本切分 → 向量化 → 相似度检索 → 重排序 → 上下文注入
//
// ## 3. 向量存储抽象
//
// 文件：vector.go, sqlite_search.go
// 接口：VectorStore（SQLite-vec / pgvector / Milvus 多后端）
//
// ## 4. 多租户隔离
//
// 文件：tenant.go
// [TenantScoped] 包装任意 Memory 实现，强制 tenantID 级数据隔离。
// [TenantRegistry] 管理多租户存储实例的生命周期。
//
// ## 5. 摘要生成
//
// 文件：summarizer.go
// [SummaryExtractor] 接口 + LLM 驱动的自动摘要提取。
//
// # 未来演进
//
// 当模块规模继续增长时，建议按上述五层拆分为独立子包：
//
//	memory/store/      — Episode CRUD
//	memory/rag/        — RAG 管道
//	memory/vector/     — 向量存储抽象
//	memory/tenant/     — 多租户隔离
//	memory/summarizer/ — 摘要生成
//
// 当前阶段通过接口解耦和文件命名约定维持清晰度。
package memory
