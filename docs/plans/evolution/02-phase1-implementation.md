# Phase 1 实施文档：闭环构建期（0-6 个月）

> 工作域：G1（Go 闭环集成）+ T1（TS 基础能力做深）  
> 前置条件：完成 `00-bugfix-register.md` 中的 P0-P1 修复

---

## G1：Go 端闭环集成

### G1-1：Planning 接入 ReAct Loop

#### 目标

Agent 收到复杂任务时，先分解为子任务，按依赖关系执行，而非直接进入单线程 ReAct。

#### 现状

```
capabilities.go:151  → PlanningCapable 接口         ✅ 已定义
capability_agent.go:246 → WithPlanner() 链式注入     ✅ 已实现
planning/planner.go  → LLMPlanner.Decompose()       ✅ 已实现
react_loop_core.go   → runLoop() 中无 planner 调用   ❌ 断线
```

#### 改动点

**文件**：`agentprimordia/internal/agent/react_loop_core.go`

在 `runLoop` 方法的循环开始前插入 Planning 逻辑：

```go
func (a *ReActAgent) runLoop(ctx context.Context, history []Message, startTurn int, cfg loopConfig, totalLLMLatency time.Duration, totalToolLatency time.Duration, toolCount int) (*Response, error) {
    // ===== 新增：Planning 阶段 =====
    // 注意：ReActAgent 本身不实现 PlanningCapable 接口，
    // 能力通过 CapabilityAgent 包装器注入（见 capability_agent.go）。
    // 此处通过 capCache 或类型断言获取 planner。
    if turn := startTurn; turn == 0 {
        if planner := a.getPlannerOrNil(); planner != nil {
            // 获取用户输入
            var userInput string
            for i := len(history) - 1; i >= 0; i-- {
                if history[i].Role == RoleUser {
                    userInput = history[i].Content
                    break
                }
            }
            if userInput != "" {
                plan, err := planner.GeneratePlan(ctx, userInput)
                if err == nil && len(plan.SubTasks) > 1 {
                    // 有多个子任务，按 DAG 执行
                    return a.executePlan(ctx, history, plan, cfg, totalLLMLatency, totalToolLatency, toolCount)
                }
                // 单子任务或规划失败，走正常 runLoop
            }
        }
    }
    // ===== 原有 runLoop 逻辑不变 =====
    // ...
}
```

**新增文件**：`agentprimordia/internal/agent/react_plan_executor.go`

```go
package agent

import (
    "context"
    "fmt"
    "time"
    "agentprimordia/internal/agent/planning"
)

// getPlannerOrNil 从 capCache 或类型断言获取 Planner
// ReActAgent 本身不实现 PlanningCapable，能力通过 CapabilityAgent 注入
func (a *ReActAgent) getPlannerOrNil() planning.Planner {
    if a.capCache != nil {
        // 如果 capCache 中有 planner（需要在 capCache 中新增 planner 字段）
        // return a.capCache.planner
    }
    // 或者通过类型断言：
    // if ca, ok := interface{}(a).(PlanningCapable); ok { return ca.GetPlanner() }
    return nil
}

// executePlan 按 DAG 依赖关系执行子任务计划
func (a *ReActAgent) executePlan(ctx context.Context, history []Message, plan *planning.Plan, cfg loopConfig, totalLLMLatency time.Duration, totalToolLatency time.Duration, toolCount int) (*Response, error) {
    // 构建依赖图
    graph := a.buildDependencyGraph(plan.SubTasks)
    
    // 拓扑排序，按层级执行
    layers := graph.TopologicalLayers()
    
    var lastOutput string
    for _, layer := range layers {
        // 同层子任务可并行（无依赖关系）
        if len(layer) == 1 {
            // 单任务，直接用 runLoop 执行
            output, err := a.executeSubTask(ctx, layer[0], history, cfg)
            if err != nil {
                return nil, fmt.Errorf("subtask %s failed: %w", layer[0].ID, err)
            }
            lastOutput = output
        } else {
            // 多任务并行执行
            outputs, err := a.executeSubTasksParallel(ctx, layer, history, cfg)
            if err != nil {
                return nil, err
            }
            lastOutput = a.mergeOutputs(outputs)
        }
    }
    
    return &Response{
        RequestID: cfg.requestID,
        Content:   lastOutput,
        Metrics: Metrics{
            TotalTurns:  len(plan.SubTasks),
            TotalTools:  toolCount,
            Duration:    time.Since(a.startTime),
            LLMLatency:  totalLLMLatency,
            ToolLatency: totalToolLatency,
        },
    }, nil
}

// executeSubTask 执行单个子任务（复用 runLoop）
func (a *ReActAgent) executeSubTask(ctx context.Context, task planning.SubTask, history []Message, cfg loopConfig) (string, error) {
    // 将子任务描述作为新的用户消息
    taskHistory := append([]Message{}, history...)
    taskHistory = append(taskHistory, UserMessage(task.Description))
    
    // 使用子 Agent 或当前 Agent 执行
    resp, err := a.runLoop(ctx, taskHistory, 0, cfg, 0, 0, 0)
    if err != nil {
        return "", err
    }
    return resp.Content, nil
}
```

#### 验收标准

```bash
# 单元测试：Planning 接入后，复杂任务被分解为子任务
go test -run TestReActAgent_WithPlanner ./internal/agent/

# 集成测试：多子任务按依赖顺序执行
go test -run TestReActAgent_PlanExecution ./internal/agent/

# 确保不影响无 Planner 的场景
go test -run TestReActAgent_WithoutPlanner ./internal/agent/
```

---

### G1-2：Reflection 接入完成路径

#### 目标

Agent 产出最终回复后，自动进行质量检查，发现高严重度问题时自动改进。

#### 改动点

**文件**：`agentprimordia/internal/agent/react_loop_core.go`

在 `len(thought.ToolCalls) == 0` 分支（L217）返回前插入 Reflection：

```go
// 无工具调用 → Agent 完成
if len(thought.ToolCalls) == 0 {
    finalContent := thought.Content
    
    // ===== 新增：Reflection 阶段 =====
    // 注意：与 Planning 相同，Reflector 通过 CapabilityAgent 注入
    if reflector := a.getReflectorOrNil(); reflector != nil && finalContent != "" {
        improved, err := a.reflectAndImprove(ctx, finalContent)
        if err == nil && improved != "" {
            finalContent = improved
            a.logger.Debug("Reflection 改进了输出", "original_len", len(thought.Content), "improved_len", len(improved))
        }
    }
    
    response := &Response{
        RequestID: cfg.requestID,
        Content:   finalContent,  // ← 使用改进后的内容
        // ... 其余字段不变
    }
    // ... 原有返回逻辑
}
```

**新增文件**：`agentprimordia/internal/agent/react_reflect.go`

```go
package agent

import (
    "context"
    "agentprimordia/internal/agent/reflection"
)

// reflectAndImprove 对输出进行反思和改进
func (a *ReActAgent) reflectAndImprove(ctx context.Context, output string) (string, error) {
    reflector := a.getReflectorOrNil()
    if reflector == nil {
        return "", nil
    }
    
    // 1. 批评当前输出
    critique, err := reflector.Critique(ctx, output)
    if err != nil {
        a.logger.Warn("Reflection critique 失败", "error", err)
        return "", err
    }
    
    // 2. 只有高严重度问题才触发改进
    if critique.Severity != reflection.SeverityHigh && critique.Severity != reflection.SeverityCritical {
        return "", nil  // 低严重度不改进
    }
    
    // 3. 基于批评改进输出
    improved, err := reflector.Improve(ctx, output, critique)
    if err != nil {
        a.logger.Warn("Reflection improve 失败", "error", err)
        return "", err
    }
    
    return improved, nil
}
```

#### 验收标准

```bash
go test -run TestReActAgent_WithReflection ./internal/agent/
go test -run TestReflectAndImprove_LowSeverity ./internal/agent/
go test -run TestReflectAndImprove_HighSeverity ./internal/agent/
```

---

### G1-3：ToolLearning 接入工具执行

#### 目标

工具执行前查询历史经验优化参数，执行后记录结果。

#### 改动点

**文件**：`agentprimordia/internal/agent/react_loop_tools.go`

在 `executeToolCalls` 的循环内，`executeTool` 调用前后插入学习逻辑：

```go
func (a *ReActAgent) executeToolCalls(ctx context.Context, history []Message, toolCalls []ToolCall, turn int, cfg loopConfig, tracer Tracer, turnSpan Span, totalToolLatency time.Duration, toolCount int) ([]Message, time.Duration, int) {
    learner := a.getToolLearnerOrNil()  // 新增：获取 learner（通过 capCache 或类型断言）
    
    for _, tc := range toolCalls {
        // ... 原有 emitStream / fireHook / HITL 逻辑不变 ...
        
        // ===== 新增：工具执行前 — 查询历史经验 =====
        if learner != nil {
            suggestion, err := learner.SuggestImprovement(ctx, tc.Name, tc.Args)
            if err == nil && suggestion != nil && suggestion.Confidence > 0.7 {
                a.logger.Debug("ToolLearning 建议优化参数", "tool", tc.Name, "confidence", suggestion.Confidence)
                tc.Args = suggestion.ImprovedArgs
            }
        }
        
        // 原有 executeTool 调用
        result, err := a.executeTool(ctx, tc)
        
        // ===== 新增：工具执行后 — 记录经验 =====
        if learner != nil {
            if err != nil || result.IsError {
                _ = learner.RecordFailure(ctx, tc.Name, tc.Args, func() string {
                    if err != nil {
                        return err.Error()
                    }
                    return result.Content
                }())
            } else {
                _ = learner.RecordSuccess(ctx, tc.Name, tc.Args, result.Content)
            }
        }
        
        // ... 原有后续逻辑不变 ...
    }
    return history, totalToolLatency, toolCount
}
```

**同步修复**：`tool_learning/learner.go` 的 `GetBestPractices()` 空实现（BUG-04）。

#### 验收标准

```bash
go test -run TestExecuteToolCalls_WithLearning ./internal/agent/
go test -run TestToolLearner_RecordAndSuggest ./internal/agent/tool_learning/
```

---

### G1-4：并行工具执行

#### 目标

同一轮的多个工具调用并行执行，利用 goroutine 降低延迟。

#### 改动点

**文件**：`agentprimordia/internal/agent/react_loop_tools.go`

新增 `executeToolCallsParallel` 方法，在 `executeToolCalls` 入口处根据配置选择串行或并行：

```go
import "golang.org/x/sync/errgroup"

// 在 ReActConfig 中新增字段
type ReActConfig struct {
    // ... 原有字段 ...
    ParallelToolExecution bool
    MaxParallelTools      int  // 0 = 无限制
}

// executeToolCalls 入口处分流
func (a *ReActAgent) executeToolCalls(...) ([]Message, time.Duration, int) {
    if a.config.ParallelToolExecution && len(toolCalls) > 1 {
        return a.executeToolCallsParallel(ctx, history, toolCalls, turn, cfg, tracer, turnSpan, totalToolLatency, toolCount)
    }
    // 原有串行逻辑
    // ...
}

func (a *ReActAgent) executeToolCallsParallel(ctx context.Context, history []Message, toolCalls []ToolCall, turn int, cfg loopConfig, tracer Tracer, turnSpan Span, totalToolLatency time.Duration, toolCount int) ([]Message, time.Duration, int) {
    type toolExecResult struct {
        result *ToolResult
        err    error
        index  int
    }
    
    maxParallel := a.config.MaxParallelTools
    if maxParallel <= 0 {
        maxParallel = len(toolCalls)
    }
    
    // 分批并行执行
    results := make([]*ToolResult, len(toolCalls))
    
    for i := 0; i < len(toolCalls); i += maxParallel {
        end := i + maxParallel
        if end > len(toolCalls) {
            end = len(toolCalls)
        }
        batch := toolCalls[i:end]
        
        g, gctx := errgroup.WithContext(ctx)
        batchResults := make([]*ToolResult, len(batch))
        
        for j, tc := range batch {
            j, tc := j, tc
            g.Go(func() error {
                // HITL 检查
                tc, skip := a.handleHITL(gctx, &tc, turn, cfg)
                if skip {
                    batchResults[j] = &ToolResult{ToolCallID: tc.ID, Content: "人类拒绝执行此操作", IsError: true}
                    return nil
                }
                
                result, err := a.executeTool(gctx, tc)
                batchResults[j] = result
                return err
            })
        }
        _ = g.Wait()
        
        for j := range batch {
            results[i+j] = batchResults[j]
        }
    }
    
    // 串行处理结果（保持消息顺序、触发 hook、记录学习）
    for i, tc := range toolCalls {
        result := results[i]
        // ... 原有 emitStream / fireHook / append history / saveMemory 逻辑 ...
    }
    
    return history, totalToolLatency, toolCount
}
```

#### 依赖

```
# go.mod 新增
require golang.org/x/sync v0.10.0
```

#### 验收标准

```bash
go test -race -run TestExecuteToolCalls_Parallel ./internal/agent/
go test -bench BenchmarkExecuteToolCalls_Parallel ./internal/agent/
```

---

### G1-5：BUG 修复（前置）

在以上功能开发前，先完成 `00-bugfix-register.md` 中的修复：

| 顺序 | BUG | 预计工时 |
|------|-----|---------|
| 1 | BUG-01 并发测试数据竞争 | 15 分钟 |
| 2 | BUG-02 O(n²) 字符串拼接 | 10 分钟 |
| 3 | BUG-03 错误链断裂 | 2 小时（28 处，需逐个审查） |
| 4 | BUG-04 GetBestPractices 空实现 | 1 小时 |
| 5 | BUG-07 Go 工具链升级 | 30 分钟 |

---

## T1：TS 端基础能力做深

### T1-1：实现真正的 HNSW

#### 目标

将 `vector-extended.ts` 的 insert 从 O(n log n) 改为真正的 O(log n) 分层图搜索。

#### 现状

```typescript
// vector-extended.ts:62-66 — 当前实现
for (let l = 0; l <= Math.min(level, this.maxLevel); l++) {
    const candidates = Array.from(this.nodes.values())  // 遍历全部节点
        .filter(n => n.id !== id && n.level >= l)
        .sort(...)  // O(n log n) 排序
        .slice(0, this.config.maxConnections);
}
```

#### 改动方案

**文件**：`sdk/typescript/src/memory/vector-extended.ts`

重写 `insert` 和 `search` 方法，实现真正的分层贪心搜索：

```typescript
export class HNSW {
  // ... 原有字段不变 ...

  insert(id: string, vector: number[]): void {
    const level = this.randomLevel();
    const node: HNSWNode = { id, vector, level, neighbors: new Map() };
    for (let l = 0; l <= level; l++) node.neighbors.set(l, []);

    if (!this.entryPoint) {
      this.entryPoint = id;
      this.maxLevel = level;
      this.nodes.set(id, node);
      return;
    }

    // 从顶层贪心搜索到插入层
    let current = this.entryPoint;
    for (let l = this.maxLevel; l > level; l--) {
      current = this.greedySearchLayer(vector, current, l);
    }

    // 在插入层及以下建立连接
    for (let l = Math.min(level, this.maxLevel); l >= 0; l--) {
      // 在当前层搜索 efConstruction 个最近邻
      const candidates = this.searchLayer(vector, current, this.config.efConstruction, l);
      
      // 从候选中选择 maxConnections 个最近的
      const selected = candidates
        .sort((a, b) => this.distance(vector, this.nodes.get(a.id)!.vector) - 
                          this.distance(vector, this.nodes.get(b.id)!.vector))
        .slice(0, this.config.maxConnections);

      // 建立双向连接
      node.neighbors.set(l, selected.map(c => c.id));
      for (const c of selected) {
        const cn = this.nodes.get(c.id)!;
        cn.neighbors.get(l)!.push(id);
        // 裁剪邻居数量
        this.pruneNeighbors(c.id, l);
      }

      current = candidates[0]?.id ?? current;
    }

    if (level > this.maxLevel) {
      this.maxLevel = level;
      this.entryPoint = id;
    }
    this.nodes.set(id, node);
  }

  // 单层贪心搜索：找到该层最近的节点
  private greedySearchLayer(query: number[], entryPoint: string, layer: number): string {
    let current = entryPoint;
    let currentDist = this.distance(query, this.nodes.get(current)!.vector);
    let improved = true;
    
    while (improved) {
      improved = false;
      const neighbors = this.nodes.get(current)!.neighbors.get(layer) ?? [];
      for (const neighborId of neighbors) {
        const dist = this.distance(query, this.nodes.get(neighborId)!.vector);
        if (dist < currentDist) {
          current = neighborId;
          currentDist = dist;
          improved = true;
        }
      }
    }
    return current;
  }

  // 单层 ef 搜索：返回 ef 个候选
  // 注意：此处使用数组 + sort + shift 实现，复杂度为 O(n) 而非 O(log n)。
  // 生产环境应替换为最小堆（MinHeap）以获得真正的 O(log n) 性能。
  private searchLayer(query: number[], entryPoint: string, ef: number, layer: number): { id: string; dist: number }[] {
    const visited = new Set<string>([entryPoint]);
    // TODO: 替换为 MinHeap 实现，当前用 sorted array 作为简化方案
    const candidates: { id: string; dist: number }[] = [
      { id: entryPoint, dist: this.distance(query, this.nodes.get(entryPoint)!.vector) }
    ];
    const results = [...candidates];

    while (candidates.length > 0) {
      // 取最近的候选（生产环境应使用 MinHeap.pop()）
      candidates.sort((a, b) => a.dist - b.dist);
      const current = candidates.shift()!;
      
      // 检查邻居
      const neighbors = this.nodes.get(current.id)!.neighbors.get(layer) ?? [];
      for (const neighborId of neighbors) {
        if (visited.has(neighborId)) continue;
        visited.add(neighborId);
        const dist = this.distance(query, this.nodes.get(neighborId)!.vector);
        
        if (results.length < ef || dist < results[results.length - 1]!.dist) {
          candidates.push({ id: neighborId, dist });
          results.push({ id: neighborId, dist });
          results.sort((a, b) => a.dist - b.dist);
          if (results.length > ef) results.pop();
        }
      }
    }
    return results;
  }

  private pruneNeighbors(nodeId: string, layer: number): void {
    const node = this.nodes.get(nodeId)!;
    const neighbors = node.neighbors.get(layer) ?? [];
    const maxConn = layer === 0 ? this.config.maxConnectionsLayer0 : this.config.maxConnections;
    if (neighbors.length <= maxConn) return;
    
    neighbors.sort((a, b) => 
      this.distance(node.vector, this.nodes.get(a)!.vector) - 
      this.distance(node.vector, this.nodes.get(b)!.vector)
    );
    node.neighbors.set(layer, neighbors.slice(0, maxConn));
  }
}
```

#### 验收标准

```bash
cd sdk/typescript
npm test -- --grep "HNSW"
# 性能测试：10000 向量 insert 应 < 500ms（之前 > 5s）
npm run bench -- vector-extended
```

---

### T1-2：浏览器端向量存储

#### 目标

支持在浏览器中使用 IndexedDB 持久化向量数据。

#### 新增文件

```
sdk/typescript/src/memory/
  indexeddb-vector-store.ts   ← 新增
```

```typescript
export class IndexedDBVectorStore implements VectorStore {
  private dbName = 'agentprimordia';
  private storeName = 'vectors';
  private db: IDBDatabase | null = null;

  async init(): Promise<void> {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(this.dbName, 1);
      req.onupgradeneeded = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains(this.storeName)) {
          db.createObjectStore(this.storeName, { keyPath: 'id' });
        }
      };
      req.onsuccess = () => { this.db = req.result; resolve(); };
      req.onerror = () => reject(req.error);
    });
  }

  async add(id: string, vector: Float32Array, metadata?: Record<string, unknown>): Promise<void> {
    // IndexedDB 可以直接存储 Float32Array（结构化克隆）
    // 注意：原生 IndexedDB 事务没有 .done 属性，以下使用 idb 库的 API
    // 如果不使用 idb 库，需要用 tx.oncomplete 回调
    const tx = this.db!.transaction(this.storeName, 'readwrite');
    tx.objectStore(this.storeName).put({ id, vector, metadata });
    await tx.done; // idb 库 API，原生需替换为 Promise + oncomplete
  }

  async search(query: Float32Array, k: number): Promise<VectorSearchResult[]> {
    const tx = this.db!.transaction(this.storeName, 'readonly');
    const all = await tx.objectStore(this.storeName).getAll();
    
    // 暴力搜索（小规模数据集可接受）
    const results = all.map((item: any) => ({
      id: item.id,
      score: this.cosineSimilarity(query, item.vector),
      metadata: item.metadata,
    }));
    
    return results.sort((a, b) => b.score - a.score).slice(0, k);
  }
}
```

#### 验收标准

```bash
npm test -- --grep "IndexedDBVectorStore"
```

---

### T1-3：流式 tool_calls 实时解析

#### 目标

在 LLM 流式输出中实时识别 tool_calls JSON，而非等全部 token 收完再解析。

#### 改动点

**文件**：`sdk/typescript/src/agent/react-reasoning.ts`

在 `reasonStream` 方法中，使用增量 JSON 解析器：

```typescript
export class ReasoningEngine {
  // 新增：增量 JSON 解析状态
  private jsonBuffer = '';
  private inToolCall = false;

  async reasonStream(messages: Message[], toolDefs: ToolDefinition[]): Promise<Thought> {
    // ... 原有流式初始化 ...
    
    let contentBuffer = '';
    for await (const chunk of this.config.provider.stream(req)) {
      if (chunk.content) {
        // 检测是否进入 tool_call JSON 片段
        if (this.detectToolCallStart(chunk.content)) {
          this.inToolCall = true;
          this.jsonBuffer = '';
        }
        
        if (this.inToolCall) {
          this.jsonBuffer += chunk.content;
          // 尝试增量解析
          const parsed = this.tryParsePartialToolCall();
          if (parsed) {
            this.inToolCall = false;
            return { content: contentBuffer, toolCalls: [parsed] };
          }
        } else {
          contentBuffer += chunk.content;
          this.emit({ type: 'token', content: chunk.content });
        }
      }
      if (chunk.done) break;
    }
    // ... 原有后续逻辑 ...
  }
}
```

#### 验收标准

```bash
npm test -- --grep "StreamToolCall"
```

---

## Phase 1 里程碑

| 里程碑 | 时间 | 交付物 |
|--------|------|--------|
| M1.1 | 第 2 周 | BUG-01/02/03 修复，CI 恢复绿色 |
| M1.2 | 第 4 周 | G1-1 Planning 接入，G1-2 Reflection 接入 |
| M1.3 | 第 6 周 | G1-3 ToolLearning 接入，G1-4 并行工具 |
| M1.4 | 第 8 周 | T1-1 HNSW 重写 |
| M1.5 | 第 10 周 | T1-2 浏览器向量存储，T1-3 流式解析 |
| M1.6 | 第 12 周 | 端到端测试 + 性能基准 |

## Phase 1 验收标准

### Go 端

```bash
# 全量测试通过
go test -race ./...
# Planning/Reflection/ToolLearning 闭环测试
go test -run TestPlanReflectLearn ./internal/agent/
# 并行工具性能基准
go test -bench BenchmarkExecuteToolCalls ./internal/agent/
```

### TS 端

```bash
cd sdk/typescript
npm test
# HNSW 性能基准
npm run bench -- vector-extended
# 浏览器存储测试
npm test -- --grep "IndexedDB"
```
