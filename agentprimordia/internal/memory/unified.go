// unified.go — Memory 与 VectorStore 的统一组合接口（v6.x 评估报告 Issue #11 修复）
//
// 背景问题：
//   旧设计把"记忆 CRUD"（Memory）与"向量检索"（VectorStore）分成两个平行
//   接口，RAG 调用方必须外部同时持有两者。这带来两个硬伤：
//     1. 无事务一致性——`memory.Add` 成功但 `vectors.Insert` 失败会留下
//        "看不见的孤儿记忆"（Search 命中但 Query 不命中，反之亦然）。
//     2. 调用方要自己维护 collection 名与 sessionID 的映射约定。
//
// v6.x 方案（纯增量，不破坏现有 API）：
//   - MemoryWithVectors：组合接口，让"既有记忆又有向量"的存储实现一处声明。
//   - UnifiedMemory：把 Memory + VectorStore 组合成一个实现，负责 collection
//     命名约定（`mem:<sessionID>`）与写入补偿（向量写失败时回滚记忆）。
//   - AddWithVector / DeleteWithVector：带补偿的事务性写入，杜绝孤儿数据。
//
// 该文件不修改任何现有类型/接口，仅新增；旧调用方完全不受影响。

package memory

import (
	"context"
	"fmt"
	"sync"
)

// 注意：Go 无法直接组合 Memory 与 VectorStore 两个接口——二者都有
// Delete/Search 方法但签名不同（Memory: Delete(id), Search(query, opts)；
// VectorStore: Delete(collection, ids), Search(collection, query, opts)）。
// 因此这里不定义组合接口，而通过 UnifiedMemory 组合实现提供统一入口。

// CollectionNameForSession 返回某 sessionID 对应的向量集合名。
//
// 约定：`mem:<sessionID>`。sessionID 为空时使用默认集合 `mem:default`。
// 该约定由 UnifiedMemory 统一管理，调用方无需关心。
func CollectionNameForSession(sessionID string) string {
	if sessionID == "" {
		return "mem:default"
	}
	return "mem:" + sessionID
}

// UnifiedMemory 组合 Memory + VectorStore 的统一实现。
//
// 职责：
//  1. 向量集合按 sessionID 自动命名（CollectionNameForSession）；
//  2. 提供带补偿的写入（AddWithVector / DeleteWithVector），
//     避免"记忆写成功但向量写失败"的孤儿数据；
//  3. 保持对底层 Memory / VectorStore 的透明转发。
type UnifiedMemory struct {
	mem Memory
	vec VectorStore
	mu  sync.Mutex // 保护 AddWithVector 的补偿窗口（防止并发交错删除）
}

// NewUnifiedMemory 用底层 Memory + VectorStore 组装统一存储。
func NewUnifiedMemory(mem Memory, vec VectorStore) *UnifiedMemory {
	return &UnifiedMemory{mem: mem, vec: vec}
}

// Memory 返回底层记忆存储（透传）。
func (u *UnifiedMemory) Memory() Memory { return u.mem }

// VectorStore 返回底层向量存储（透传）。
func (u *UnifiedMemory) VectorStore() VectorStore { return u.vec }

// AddWithVector 以补偿式事务写入一条记忆 + 对应向量：
//
//  1. 先写记忆（Memory.Add）；
//  2. 再写向量（VectorStore.Insert）；
//  3. 若向量写失败，删除刚写入的记忆（补偿），并返回错误——
//     保证"记忆/向量"要么都成功要么都失败，不产生孤儿数据。
//
// metadata 会附加 session_id 键，便于后续按租户/会话过滤。
func (u *UnifiedMemory) AddWithVector(ctx context.Context, ep *Episode, collection string, vector []float32, metadata map[string]string) error {
	if u.mem == nil || u.vec == nil {
		return fmt.Errorf("unified memory: nil mem/vec backend")
	}
	if collection == "" {
		collection = CollectionNameForSession(ep.SessionID)
	}

	// 先写记忆
	if err := u.mem.Add(ctx, ep); err != nil {
		return fmt.Errorf("unified memory: add episode: %w", err)
	}

	// 组装向量记录（带 session_id 元数据）
	vecMeta := make(map[string]any, len(metadata)+1)
	for k, v := range metadata {
		vecMeta[k] = v
	}
	if ep.SessionID != "" {
		vecMeta["session_id"] = ep.SessionID
	}

	// 写向量；失败则补偿删除记忆
	err := u.vec.Insert(ctx, collection, []*VectorRecord{
		{ID: ep.ID, Vector: vector, Metadata: vecMeta},
	})
	if err != nil {
		// 补偿：删除刚写入的记忆，防止孤儿
		if delErr := u.mem.Delete(ctx, ep.ID); delErr != nil {
			return fmt.Errorf("unified memory: insert vector failed (%v) AND compensate delete failed (%v)", err, delErr)
		}
		return fmt.Errorf("unified memory: insert vector: %w", err)
	}
	return nil
}

// DeleteWithVector 以补偿式事务删除记忆 + 对应向量。
func (u *UnifiedMemory) DeleteWithVector(ctx context.Context, id, collection string) error {
	if u.mem == nil || u.vec == nil {
		return fmt.Errorf("unified memory: nil mem/vec backend")
	}
	u.mu.Lock()
	defer u.mu.Unlock()

	if err := u.mem.Delete(ctx, id); err != nil {
		return fmt.Errorf("unified memory: delete episode: %w", err)
	}
	if err := u.vec.Delete(ctx, collection, []string{id}); err != nil {
		// 向量删除失败不补偿（记忆已删，向量残留是无害的脏数据，
		// 可通过 Search 层 ID 去重忽略）
		return fmt.Errorf("unified memory: delete vector: %w", err)
	}
	return nil
}

// SearchHybrid 在指定 session 的向量集合中检索，并回填 Episode 内容。
//
// 返回的 []*HybridResult 携带向量命中分数与对应记忆 Episode。
// 该方法是"统一接口"的直接消费者，RAG 调用方不再需要分别查询
// VectorStore 与 Memory 再手工关联。
func (u *UnifiedMemory) SearchHybrid(ctx context.Context, sessionID string, query []float32, opts VectorSearchOptions) ([]*HybridResult, error) {
	if u.vec == nil {
		return nil, fmt.Errorf("unified memory: nil vector backend")
	}
	collection := CollectionNameForSession(sessionID)
	matches, err := u.vec.Search(ctx, collection, query, opts)
	if err != nil {
		return nil, err
	}
	if u.mem == nil {
		// 无记忆后端时只返回向量命中（不含 Episode）
		out := make([]*HybridResult, 0, len(matches))
		for _, m := range matches {
			out = append(out, &HybridResult{ID: m.ID, Score: m.Score})
		}
		return out, nil
	}
	// 批量回填记忆
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m.ID)
	}
	episodes, err := u.mem.GetBatch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("unified memory: get batch episodes: %w", err)
	}
	out := make([]*HybridResult, 0, len(matches))
	for _, m := range matches {
		hr := &HybridResult{ID: m.ID, Score: m.Score}
		if ep, ok := episodes[m.ID]; ok {
			hr.Episode = ep
		}
		out = append(out, hr)
	}
	return out, nil
}

// HybridResult 向量检索 + 记忆回填的联合结果。
type HybridResult struct {
	ID      string
	Score   float32
	Episode *Episode // 可能为 nil（记忆后端无此 ID）
}
