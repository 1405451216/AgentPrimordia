# Phase 2 实施文档：自主进化期（6-12 个月）

> 工作域：G2（Go 高级能力 + 分布式）+ T2（TS 开发体验平台化）  
> 前置条件：Phase 1 全部完成

> **进度更新 2026-07-09（终验）**：本阶段 G2-1、G2-2、G2-3、G2-4、G2-5 与 T2-1、T2-2、T2-3、T2-4 **全部已在代码落地并通过全量验证**。权威结论见同目录 `EXECUTION_STATUS.md`。注：G2-3（etcd/redis）与 G2-4（Operator v2）引入的白名单外依赖需维护者按 AGENTS.md §2.2 决策。

---

## G2：Go 端高级能力与分布式

### G2-1：成本感知模型路由器

#### 目标

Go 端实现无锁统计的模型路由器，根据任务复杂度和成本自动选择模型。

#### 背景

TS 端已有 `model-router.ts`，Go 端无对应实现。Go 版本利用 `atomic` 做无锁统计，性能优于 TS。

#### 新增文件

```
agentprimordia/internal/llm/model_router.go
agentprimordia/internal/llm/model_router_test.go
```

#### 核心设计

```go
package llm

import (
    "context"
    "math"
    "sync/atomic"
)

// ModelRouteConfig 模型路由配置
type ModelRouteConfig struct {
    Name           string
    Provider       Provider
    CostPer1K      float64
    ComplexityLimit float64  // [0,1]
    MaxContext     int
    SupportsTools  bool
    Priority       int       // 数字越小优先级越高
}

// modelStats 无锁统计（每个模型一组原子计数器）
type modelStats struct {
    calls     atomic.Int64
    failures  atomic.Int64
    totalMs   atomic.Int64
    totalCost atomic.Uint64 // 用 uint64 存 float64 的 bits，避免 Float64 兼容性问题
}

// ModelRouter 智能模型路由器
// 注意：Register 必须在 Route 调用前完成（初始化阶段），运行时不可并发注册
// 如需运行时动态注册，需加 sync.RWMutex 保护 models 切片
type ModelRouter struct {
    models    []modelEntry
    strategy  RouteStrategy
    fallback  string
}

type modelEntry struct {
    config ModelRouteConfig
    stats  modelStats
}

func NewModelRouter(strategy RouteStrategy) *ModelRouter {
    return &ModelRouter{strategy: strategy}
}

func (r *ModelRouter) Register(cfg ModelRouteConfig) {
    r.models = append(r.models, modelEntry{config: cfg})
}

// Route 根据消息复杂度选择最优模型
func (r *ModelRouter) Route(messages []ChatMessage, hasTools bool) (Provider, *RouteDecision, error) {
    complexity := r.evaluateComplexity(messages)
    contextLen := estimateTokens(messages)
    
    candidates := r.filterCandidates(complexity, contextLen, hasTools)
    if len(candidates) == 0 {
        if r.fallback != "" {
            // 降级到 fallback
        }
        return nil, nil, ErrNoSuitableModel
    }
    
    // 按策略排序
    switch r.strategy {
    case StrategyCostFirst:
        // 选最便宜的
    case StrategyQualityFirst:
        // 选能力最强的
    case StrategyBalanced:
        // 平衡成本和质量
    }
    
    selected := candidates[0]
    decision := &RouteDecision{
        ModelName:    selected.config.Name,
        Complexity:   complexity,
        EstimatedCost: selected.config.CostPer1K * float64(contextLen) / 1000,
    }
    return selected.config.Provider, decision, nil
}

// evaluateComplexity 评估消息复杂度 [0,1]
func (r *ModelRouter) evaluateComplexity(messages []ChatMessage) float64 {
    var totalLen int
    var hasCode, hasMultistep, hasReasoning bool
    
    for _, msg := range messages {
        totalLen += len(msg.Content)
        // 启发式检测
        if containsAny(msg.Content, "代码", "code", "function", "实现") {
            hasCode = true
        }
        if containsAny(msg.Content, "步骤", "step", "首先", "然后") {
            hasMultistep = true
        }
        if containsAny(msg.Content, "为什么", "分析", "推理", "explain") {
            hasReasoning = true
        }
    }
    
    complexity := 0.0
    if totalLen > 4000 { complexity += 0.3 }
    if totalLen > 8000 { complexity += 0.2 }
    if hasCode { complexity += 0.2 }
    if hasMultistep { complexity += 0.15 }
    if hasReasoning { complexity += 0.15 }
    
    return math.Min(complexity, 1.0)
}

// 注意：以下辅助函数需自行实现：
// - containsAny(s string, keywords ...string) bool
// - estimateTokens(messages []ChatMessage) int
// - filterCandidates(complexity float64, contextLen int, hasTools bool) []modelEntry
```

#### 验收标准

```bash
go test -race -run TestModelRouter ./internal/llm/
go test -bench BenchmarkModelRouter ./internal/llm/
# 确认无锁统计在并发下不丢失数据
go test -race -count=50 -run TestModelRouter_ConcurrentStats ./internal/llm/
```

---

### G2-2：Go 原生投机执行

#### 目标

工具执行期间并行启动预测性 LLM 调用，命中则节省一轮 LLM 延迟。

#### 背景

TS 端有 `speculative-exec.ts`（基于 Promise.race）。Go 版本利用 `select` + channel 做更灵活的取消控制。

#### 新增文件

```
agentprimordia/internal/agent/speculative_exec.go
agentprimordia/internal/agent/speculative_exec_test.go
```

#### 核心设计

```go
package agent

import (
    "context"
    "time"
    "agentprimordia/internal/llm"
)

// SpeculativeExecutor 投机执行器
type SpeculativeExecutor struct {
    provider llm.Provider
    predictor *ToolResultPredictor
    minHitRate float64
    stats SpeculationStats
}

type ToolResultPredictor struct {
    history map[string][]ToolResult // toolName -> 最近结果
    // 注意：history 并发读写需加锁保护，Predict 和 Record 不能并发调用
    // 生产环境应使用 sync.RWMutex 或 channel 串行化访问
    mu sync.RWMutex
}

func (p *ToolResultPredictor) Predict(toolName, args string) (*ToolResult, bool) {
    p.mu.RLock()
    defer p.mu.RUnlock()
    records := p.history[toolName]
    if len(records) == 0 {
        return nil, false
    }
    // 简单策略：使用最近一次成功结果
    for i := len(records) - 1; i >= 0; i-- {
        if !records[i].IsError {
            return &records[i], true
        }
    }
    return nil, false
}

// ExecuteWithSpeculation 工具执行 + 投机 LLM 预测
func (e *SpeculativeExecutor) ExecuteWithSpeculation(
    ctx context.Context,
    messages []llm.ChatMessage,
    toolCalls []ToolCall,
    executeTool func(context.Context, ToolCall) (*ToolResult, error),
) (*SpeculativeResult, error) {
    
    // 1. 预测工具结果
    predictions := make([]predictedResult, len(toolCalls))
    canSpeculate := false
    for i, tc := range toolCalls {
        pred, ok := e.predictor.Predict(tc.Name, tc.Args)
        predictions[i] = predictedResult{tc: tc, predicted: pred}
        if ok { canSpeculate = true }
    }
    
    // 2. 启动实际工具执行
    toolResultCh := make(chan toolExecOutcome, 1)
    go func() {
        results := make([]*ToolResult, len(toolCalls))
        for i, tc := range toolCalls {
            r, err := executeTool(ctx, tc)
            results[i] = r
            if err != nil {
                toolResultCh <- toolExecOutcome{results: results, err: err}
                return
            }
        }
        toolResultCh <- toolExecOutcome{results: results}
    }()
    
    // 3. 如果可以投机，同时启动预测性 LLM 调用
    var specCh chan *llm.ToolCallResponse
    if canSpeculate {
        specCh = make(chan *llm.ToolCallResponse, 1)
        specMessages := e.buildSpeculativeMessages(messages, toolCalls, predictions)
        go func() {
            resp, err := e.provider.CallTools(ctx, &llm.ToolCallRequest{
                Messages: specMessages,
            })
            if err == nil {
                specCh <- resp
            } else {
                close(specCh)
            }
        }()
    }
    
    // 4. 等待工具执行完成
    outcome := <-toolResultCh
    if outcome.err != nil {
        return nil, outcome.err
    }
    
    // 5. 记录工具结果用于未来预测
    for i, result := range outcome.results {
        e.predictor.Record(toolCalls[i].Name, *result)
    }
    
    // 6. 检查投机是否命中
    if specCh != nil {
        select {
        case specResp := <-specCh:
            // 投机完成，检查是否命中
            hit := e.checkHit(predictions, outcome.results)
            return &SpeculativeResult{
                Response:       specResp,
                SpeculationHit: hit,
                ToolResults:    outcome.results,
            }, nil
        default:
            // 投机还没完成，不等待（或设置超时）
        }
    }
    
    // 7. 投机未命中或不可用，正常返回
    return &SpeculativeResult{
        SpeculationHit: false,
        ToolResults:    outcome.results,
    }, nil
}
```

#### 验收标准

```bash
go test -race -run TestSpeculativeExec ./internal/agent/
go test -bench BenchmarkSpeculativeExec ./internal/agent/
```

---

### G2-3：分布式检查点

#### 目标

将 Agent 状态从单机 SQLite 升级为分布式存储，支持跨节点恢复。

#### 新增文件

```
agentprimordia/internal/persist/
    etcd_checkpoint.go        ← 新增
    redis_checkpoint.go       ← 新增
    distributed_test.go       ← 新增
```

#### 核心设计

```go
// persist/etcd_checkpoint.go
package persist

import (
    "context"
    "encoding/json"
    "time"
    "go.etcd.io/etcd/client/v3"
)

type EtcdCheckpointStore struct {
    client  *clientv3.Client
    prefix  string
    ttl     time.Duration
}

func NewEtcdCheckpointStore(endpoints []string, prefix string) (*EtcdCheckpointStore, error) {
    client, err := clientv3.New(clientv3.Config{
        Endpoints:   endpoints,
        DialTimeout: 5 * time.Second,
    })
    if err != nil {
        return nil, err
    }
    return &EtcdCheckpointStore{
        client: client,
        prefix: prefix,
        ttl:    24 * time.Hour,
    }, nil
}

func (s *EtcdCheckpointStore) Save(ctx context.Context, state *AgentState) error {
    data, err := json.Marshal(state)
    if err != nil {
        return err
    }
    key := s.prefix + "/" + state.AgentID
    
    // 带租约保存，自动过期
    lease, err := s.client.Grant(ctx, int64(s.ttl.Seconds()))
    if err != nil {
        return err
    }
    _, err = s.client.Put(ctx, key, string(data), clientv3.WithLease(lease.ID))
    return err
}

func (s *EtcdCheckpointStore) Load(ctx context.Context, agentID string) (*AgentState, error) {
    key := s.prefix + "/" + agentID
    resp, err := s.client.Get(ctx, key)
    if err != nil {
        return nil, err
    }
    if len(resp.Kvs) == 0 {
        return nil, ErrCheckpointNotFound
    }
    var state AgentState
    if err := json.Unmarshal(resp.Kvs[0].Value, &state); err != nil {
        return nil, err
    }
    return &state, nil
}
```

#### 验收标准

```bash
# 需要本地 etcd 实例
go test -run TestEtcdCheckpoint ./internal/persist/
# 分布式恢复测试
go test -run TestDistributedRecovery ./internal/agent/
```

---

### G2-4：K8s Operator v2

#### 目标

扩展 CRD 支持多 Agent 流水线和集群，增加 GPU 调度和灰度发布。

#### 改动点

**文件**：`agentprimordia/operator/api/v1/types.go`

```go
// 新增 CRD 类型
type AgentPipelineSpec struct {
    Steps []PipelineStepSpec `json:"steps"`
    // 步骤间传递数据的模式
    DataTransferMode string `json:"dataTransferMode"` // "inline" | "sharedVolume" | "messageQueue"
}

type AgentSwarmSpec struct {
    Workers  int32               `json:"workers"`
    Template AgentDeploymentSpec `json:"template"`
    // 任务分配策略
    Scheduler string `json:"scheduler"` // "roundRobin" | "leastLoaded" | "affinity"
    // Agent 间通信
    Communication string `json:"communication"` // "grpc" | "redis" | "none"
}
```

**文件**：`agentprimordia/operator/controller/rolling.go` 扩展灰度发布：

```go
// 基于 Eval 结果的自动滚动发布
func (r *AgentDeploymentReconciler) rollingUpdateWithEval(
    ctx context.Context,
    agentDeploy *agentv1.AgentDeployment,
    newDeployment *appsv1.Deployment,
) error {
    // 1. 灰度发布 10% 流量到新版本
    // 2. 等待 Eval 结果
    // 3. 如果 Eval 通过，继续滚动
    // 4. 如果 Eval 失败，自动回滚
}
```

#### 验收标准

```bash
cd agentprimordia/operator
make test
# 需要 K8s 测试集群
make test-e2e
```

---

### G2-5：Eval CI 集成

#### 目标

将 Eval Suite 集成到 CI 流水线，防止 Agent 行为退化。

#### 新增文件

```
agentprimordia/bench/eval-ci/
    run_eval.sh
    eval_cases.json
```

```yaml
# .github/workflows/eval.yml
name: Agent Eval
on: [push, pull_request]
jobs:
  eval:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test -run TestEvalSuite ./internal/agent/eval/...
      - run: ./bench/eval-ci/run_eval.sh --threshold 0.8
```

---

## T2：TS 端开发体验平台化

### T2-1：可视化 Agent 构建器

#### 目标

完整的拖拽式 Agent 设计器，支持节点拖拽、实时预览、一键导出。

#### 新增文件

```
sdk/typescript/src/visual/
    AgentDesigner.tsx         ← 主组件
    nodes/
      LLMBode.tsx              ← LLM 节点（注意：文件名应为 LLMNode.tsx）
      ToolNode.tsx             ← 工具节点
      ReflectNode.tsx          ← 反思节点
      ConditionNode.tsx        ← 条件分支节点
    edges/
      DataEdge.tsx             ← 数据流边
    panels/
      ConfigPanel.tsx          ← 右侧配置面板
      PreviewPanel.tsx         ← 实时预览面板
      ExportPanel.tsx          ← 导出面板
```

#### 核心设计

```tsx
// AgentDesigner.tsx
import { ReactFlow, Background, Controls } from 'reactflow';
import { useState } from 'react';

export function AgentDesigner() {
  const [nodes, setNodes] = useState<AgentNode[]>([]);
  const [edges, setEdges] = useState<DataEdge[]>([]);

  const onDragEnd = (event: DragEvent, nodeType: string) => {
    // 拖拽创建节点
    const newNode = createNode(nodeType, event.position);
    setNodes([...nodes, newNode]);
  };

  const onExport = () => {
    // 导出为 Agent 配置 JSON
    const config = serializeToConfig(nodes, edges);
    downloadJSON(config, 'agent-config.json');
  };

  return (
    <div className="flex h-screen">
      <div className="w-48 sidebar">
        {/* 节点工具箱 */}
        <NodePalette onDragEnd={onDragEnd} />
      </div>
      <div className="flex-1">
        <ReactFlow nodes={nodes} edges={edges}>
          <Background />
          <Controls />
        </ReactFlow>
      </div>
      <div className="w-80">
        <ConfigPanel />
        <PreviewPanel />
        <ExportPanel onExport={onExport} />
      </div>
    </div>
  );
}
```

#### 验收标准

```bash
cd sdk/typescript
npm run storybook -- --stories-file src/visual/AgentDesigner.stories.tsx
```

---

### T2-2：Prompt A/B 平台化

#### 目标

将 `prompt-ab-test.ts` 升级为完整的 Prompt 实验平台。

#### 新增文件

```
sdk/typescript/src/prompt/
    experiment-manager.ts     ← 实验管理
    statistical-test.ts       ← 统计显著性检验
    prompt-registry.ts        ← Prompt 版本管理
    prompt-hot-update.ts      ← 热更新
```

#### 核心设计

```typescript
// experiment-manager.ts
export class PromptExperimentManager {
  private experiments: Map<string, Experiment> = new Map();
  
  // 多变量测试（不只是 A/B）
  async runMultivariate(
    variants: PromptVariant[],
    testCases: string[],
    evaluator: PromptEvaluator,
    options: { minSamples?: number; confidenceLevel?: number } = {},
  ): Promise<ExperimentResult> {
    const minSamples = options.minSamples ?? 30;
    const confidence = options.confidenceLevel ?? 0.95;
    
    const results: Record<string, ExperimentResult[]> = {};
    
    // 对每个测试用例，轮流测试所有变体
    for (const testCase of testCases) {
      for (const variant of variants) {
        const result = await this.runVariant(variant, testCase, evaluator);
        results[variant.name] = results[variant.name] ?? [];
        results[variant.name].push(result);
      }
    }
    
    // 统计显著性检验
    const significance = this.testSignificance(results, confidence);
    
    return {
      winner: significance.winner,
      results,
      significance,
      recommendation: significance.isSignificant 
        ? `推广 ${significance.winner} 为默认配置`
        : '差异不显著，建议增加样本量',
    };
  }
  
  // 统计显著性检验（t-test）
  private testSignificance(
    results: Record<string, ExperimentResult[]>,
    confidenceLevel: number,
  ): SignificanceResult {
    // Welch's t-test 实现
    // ...
  }
}
```

#### 验收标准

```bash
npm test -- --grep "PromptExperiment"
npm test -- --grep "StatisticalTest"
```

---

### T2-3：插件市场

#### 目标

npm 包即插件，自动发现 + 注册 + 沙箱隔离。

#### 改动点

**文件**：`sdk/typescript/src/tools/plugin-loader.ts` 扩展

```typescript
export class AgentPluginLoader {
  // 自动扫描 node_modules 中的 @agentprimordia/plugin-* 包
  async autoDiscover(): Promise<LoadedPlugin[]> {
    const packagePaths = await this.scanNodeModules('@agentprimordia/plugin-');
    const plugins: LoadedPlugin[] = [];
    
    for (const pkgPath of packagePaths) {
      try {
        const manifest = await this.loadManifest(pkgPath);
        const plugin = await import(pkgPath);
        
        // 沙箱验证
        await this.validatePlugin(plugin);
        
        // 注册工具
        if (plugin.registerTools) {
          plugin.registerTools(this.registry);
        }
        
        plugins.push({ manifest, instance: plugin, loadedAt: new Date() });
      } catch (err) {
        console.warn(`Failed to load plugin from ${pkgPath}:`, err);
      }
    }
    return plugins;
  }
  
  // 沙箱验证：检查插件是否访问了受限 API
  private async validatePlugin(plugin: AgentPlugin): Promise<void> {
    const allowedMethods = ['registerTools', 'getTools', 'init', 'destroy'];
    const actualMethods = Object.keys(plugin);
    const disallowed = actualMethods.filter(m => !allowedMethods.includes(m));
    if (disallowed.length > 0) {
      throw new Error(`Plugin contains disallowed methods: ${disallowed.join(', ')}`);
    }
  }
}
```

#### 新增文件

```
sdk/typescript/src/tools/plugin-sandbox.ts   ← Worker Thread 沙箱
sdk/typescript/src/tools/plugin-registry.ts  ← 插件注册表
```

#### 验收标准

```bash
npm test -- --grep "PluginLoader"
npm test -- --grep "PluginSandbox"
```

---

### T2-4：React 19 深度集成

#### 目标

利用 React 19 的 Server Components 和并发特性深度集成 Agent。

#### 新增文件

```
sdk/typescript/src/react/
    server-components/
      AgentServerComponent.tsx  ← RSC 兼容的 Agent 组件
    hooks/
      useAgentStream.ts         ← 流式输出 Hook
      useAgentSuspense.ts       ← Suspense 集成
```

#### 核心设计

```tsx
// useAgentStream.ts — 直接订阅 Agent 流式输出
import { useState, useEffect } from 'react';

export function useAgentStream(agent: ReActAgent) {
  const [tokens, setTokens] = useState<string[]>([]);
  const [isStreaming, setIsStreaming] = useState(false);
  
  useEffect(() => {
    const unsubscribe = agent.onStream((event: StreamEvent) => {
      if (event.type === 'token') {
        setTokens(prev => [...prev, event.content]);
      } else if (event.type === 'done') {
        setIsStreaming(false);
      }
    });
    return unsubscribe;
  }, [agent]);
  
  return { content: tokens.join(''), isStreaming };
}

// useAgentSuspense.ts — Suspense 集成
import { use } from 'react';

export function useAgentSuspense(agent: ReActAgent, input: string) {
  return use(
    agent.run(input).then(response => response.content)
  );
}

// AgentServerComponent.tsx — RSC 兼容
export async function AgentServerComponent({ input }: { input: string }) {
  const agent = createAgent('server-agent')
    .withProvider(getProvider())
    .withToolkit(getToolkit())
    .build();
  
  const response = await agent.run(input);
  
  return (
    <div>
      <p>{response.content}</p>
      <AgentMetrics metrics={response.metrics} />
    </div>
  );
}
```

#### 验收标准

```bash
npm test -- --grep "useAgentStream"
npm test -- --grep "AgentServerComponent"
```

---

## Phase 2 里程碑

| 里程碑 | 时间 | 交付物 |
|--------|------|--------|
| M2.1 | 第 16 周 | G2-1 ModelRouter + G2-2 投机执行 |
| M2.2 | 第 20 周 | G2-3 分布式检查点 + G2-4 Operator v2 |
| M2.3 | 第 24 周 | G2-5 Eval CI + T2-1 可视化构建器 |
| M2.4 | 第 28 周 | T2-2 Prompt 平台 + T2-3 插件市场 |
| M2.5 | 第 32 周 | T2-4 React 19 集成 |
| M2.6 | 第 36 周 | 端到端测试 + 性能基准 |

## Phase 2 验收标准

### Go 端

```bash
# 模型路由器无锁统计正确性
go test -race -count=50 -run TestModelRouter_ConcurrentStats ./internal/llm/
# 投机执行命中率
go test -run TestSpeculativeExec_HitRate ./internal/agent/
# 分布式检查点恢复
go test -run TestDistributedRecovery ./internal/persist/
# Operator e2e
cd operator && make test-e2e
# Eval CI 集成
./bench/eval-ci/run_eval.sh --threshold 0.8
```

### TS 端

```bash
cd sdk/typescript
npm test
# 可视化构建器渲染
npm run storybook
# 插件自动发现
npm test -- --grep "PluginAutoDiscover"
# Prompt 统计检验
npm test -- --grep "StatisticalSignificance"
```
