// demo.go — Studio 默认演示实现
//
// 让 Studio 开箱即可演示：无需真实引擎即可看到有意义的界面数据。
// 真实引擎通过 NewStudioHandler 的 With* 选项注入替换。
package studio

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ===== demoChaos =====

type demoChaos struct {
	mu          sync.Mutex
	experiments []ExperimentResult
}

func newDemoChaos() *demoChaos { return &demoChaos{} }

func (d *demoChaos) ListExperiments(_ context.Context) ([]ExperimentResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]ExperimentResult, len(d.experiments))
	copy(out, d.experiments)
	return out, nil
}

func (d *demoChaos) CreateExperiment(_ context.Context, req CreateExperimentRequest) error {
	now := time.Now()
	exp := ExperimentResult{
		Experiment: Experiment{
			Name:        req.Name,
			Hypothesis:  req.Hypothesis,
			Status:      "completed",
			Duration:    "12.5s",
			HypothesisValidated: true,
			Faults: []Fault{{
				Type:        req.FaultType,
				Description: fmt.Sprintf("demo %s 注入（演示数据，未执行真实故障）", req.FaultType),
			}},
		},
		StartTime: now.Add(-15 * time.Second).Format(time.RFC3339),
		EndTime:   now.Format(time.RFC3339),
		PreSteadyState:  SteadyState{Met: true, Message: "稳态基线正常"},
		PostSteadyState: SteadyState{Met: true, Message: "故障恢复后稳态正常"},
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.experiments = append(d.experiments, exp)
	return nil
}

// ===== demoCluster =====

type demoCluster struct{}

func newDemoCluster() *demoCluster { return &demoCluster{} }

func (d *demoCluster) Status(_ context.Context) (*ClusterStatus, error) {
	return &ClusterStatus{
		Nodes: []NodeInfo{{
			ID:           "node-demo-1",
			Address:      "127.0.0.1:8080",
			Role:         "leader",
			Status:       "online",
			Capabilities: []string{"react-loop", "tools", "memory", "webgpu"},
			LastSeen:     time.Now().Format(time.RFC3339),
		}},
		LeaderID:     "node-demo-1",
		HashRingSize: 128,
		TotalShards:  8,
	}, nil
}

// ===== demoLearning =====

type demoLearning struct{}

func newDemoLearning() *demoLearning { return &demoLearning{} }

func (d *demoLearning) Stats(_ context.Context) (*LearningStats, error) {
	return &LearningStats{
		TotalInteractions:   0,
		TotalDistilled:      0,
		TotalKnowledgeItems: 0,
	}, nil
}

func (d *demoLearning) Capabilities(_ context.Context) ([]Capability, error) {
	return nil, nil
}

func (d *demoLearning) PipelineStats(_ context.Context) (*PipelineStats, error) {
	return &PipelineStats{LastProcessTime: time.Now().Format(time.RFC3339)}, nil
}

// ===== demoMarketplace =====

// demoTemplates 预注册的内置演示模板（名称与 ecosystem/templates/ 市场模板一致）。
var demoTemplates = []AgentTemplate{
	{
		ID:          "code-reviewer",
		Name:        "Code Reviewer",
		Description: "自动审查 PR 代码：风格、安全、性能问题与改进建议",
		Version:     "1.0.0",
		Author:      "agentprimordia",
		Category:    "coding",
		Tags:        []string{"code-review", "pr", "quality"},
		Rating:      4.8,
		Downloads:   1280,
	},
	{
		ID:          "data-analyst",
		Name:        "Data Analyst",
		Description: "数据查询与可视化助手：CSV/SQL 分析、图表生成",
		Version:     "1.0.0",
		Author:      "agentprimordia",
		Category:    "analysis",
		Tags:        []string{"data", "sql", "csv", "chart"},
		Rating:      4.6,
		Downloads:   860,
	},
	{
		ID:          "research-assistant",
		Name:        "Research Assistant",
		Description: "资料检索与综述助手：RAG 增强的文档问答",
		Version:     "1.0.0",
		Author:      "agentprimordia",
		Category:    "research",
		Tags:        []string{"rag", "search", "summarize"},
		Rating:      4.5,
		Downloads:   540,
	},
}

type demoMarketplace struct{}

func newDemoMarketplace() *demoMarketplace { return &demoMarketplace{} }

func (d *demoMarketplace) SearchTemplates(_ context.Context, query, category string) ([]AgentTemplate, error) {
	out := make([]AgentTemplate, 0, len(demoTemplates))
	for _, t := range demoTemplates {
		if category != "" && t.Category != category {
			continue
		}
		if query != "" {
			hay := strings.ToLower(t.Name + " " + t.Description + " " + strings.Join(t.Tags, " "))
			if !strings.Contains(hay, strings.ToLower(query)) {
				continue
			}
		}
		out = append(out, t)
	}
	return out, nil
}

func (d *demoMarketplace) Deploy(_ context.Context, templateID string) error {
	for _, t := range demoTemplates {
		if t.ID == templateID {
			return nil
		}
	}
	return fmt.Errorf("模板 %q 不存在", templateID)
}
