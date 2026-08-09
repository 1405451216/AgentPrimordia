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

// AbortExperiment 中止匹配名称且处于 running/pending 的实验。
// demo 实现下实验立即完成，故返回 NotFound 语义的错误提示「无运行中实验」。
func (d *demoChaos) AbortExperiment(_ context.Context, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.experiments {
		if d.experiments[i].Experiment.Name == name {
			st := d.experiments[i].Experiment.Status
			if st == "running" || st == "pending" {
				d.experiments[i].Experiment.Status = "aborted"
				return nil
			}
			return fmt.Errorf("实验 %q 当前状态为 %s，无法中止", name, st)
		}
	}
	return fmt.Errorf("实验 %q 不存在", name)
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
			Shards:       8, // 8 个分片全部由该节点持有（单节点 demo）
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
	return []Capability{
		{Name: "代码审查", Description: "PR 代码风格与缺陷审查", Score: 0.63, TimesTested: 24, TimesPassed: 19},
		{Name: "数据分析", Description: "CSV/SQL 数据分析与可视化", Score: 0.64, TimesTested: 18, TimesPassed: 14},
		{Name: "对话理解", Description: "多轮对话意图理解", Score: 0.715, TimesTested: 30, TimesPassed: 25},
	}, nil
}

func (d *demoLearning) PipelineStats(_ context.Context) (*PipelineStats, error) {
	return &PipelineStats{LastProcessTime: time.Now().Format(time.RFC3339)}, nil
}

func (d *demoLearning) CapabilityHistory(_ context.Context) ([]CapabilityHistory, error) {
	// 演示能力进化趋势：三个能力各 8 个历史时间点，缓慢上升
	now := time.Now()
	names := []string{"代码审查", "数据分析", "对话理解"}
	base := []float64{0.42, 0.50, 0.61}
	step := []float64{0.03, 0.02, 0.015}
	out := make([]CapabilityHistory, 0, len(names))
	for i, name := range names {
		pts := make([]CapabilityHistoryPoint, 0, 8)
		for j := 0; j < 8; j++ {
			pts = append(pts, CapabilityHistoryPoint{
				Score:      base[i] + step[i]*float64(j),
				RecordedAt: now.Add(-time.Duration(8-j) * 24 * time.Hour).Format(time.RFC3339),
			})
		}
		out = append(out, CapabilityHistory{Name: name, History: pts})
	}
	return out, nil
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

type demoMarketplace struct {
	mu          sync.Mutex
	deployments []Deployment
	nextID      int
}

func newDemoMarketplace() *demoMarketplace {
	return &demoMarketplace{nextID: 1}
}

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

func (d *demoMarketplace) Deploy(_ context.Context, templateID string) (Deployment, error) {
	for _, t := range demoTemplates {
		if t.ID == templateID {
			d.mu.Lock()
			defer d.mu.Unlock()
			dep := Deployment{
				ID:         fmt.Sprintf("dep-%d", d.nextID),
				TemplateID: t.ID,
				Name:       t.Name,
				Version:    t.Version,
				Category:   t.Category,
				Status:     "running",
				DeployedAt: time.Now().Format(time.RFC3339),
			}
			d.nextID++
			d.deployments = append(d.deployments, dep)
			return dep, nil
		}
	}
	return Deployment{}, fmt.Errorf("模板 %q 不存在", templateID)
}

func (d *demoMarketplace) ListDeployments(_ context.Context) ([]Deployment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Deployment, len(d.deployments))
	copy(out, d.deployments)
	return out, nil
}

func (d *demoMarketplace) StopDeployment(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.deployments {
		if d.deployments[i].ID == id {
			d.deployments[i].Status = "stopped"
			return nil
		}
	}
	return fmt.Errorf("部署 %q 不存在", id)
}

func (d *demoMarketplace) StartDeployment(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.deployments {
		if d.deployments[i].ID == id {
			d.deployments[i].Status = "running"
			return nil
		}
	}
	return fmt.Errorf("部署 %q 不存在", id)
}

// ===== demoAutonomy / demoSkills / demoRealtime =====
//
// v3.3-v3.6 面板的 demo 空实现：未注入真实引擎时返回空数组
// （面板渲染空态而非 404），响应头由 handler 标 X-Data-Source: demo。

type demoAutonomy struct{}

func newDemoAutonomy() *demoAutonomy { return &demoAutonomy{} }

func (d *demoAutonomy) Goals(_ context.Context) ([]AutonomyGoal, error) {
	return []AutonomyGoal{}, nil
}

func (d *demoAutonomy) Alerts(_ context.Context) ([]AutonomyAlert, error) {
	return []AutonomyAlert{}, nil
}

type demoSkills struct{}

func newDemoSkills() *demoSkills { return &demoSkills{} }

func (d *demoSkills) List(_ context.Context) ([]SkillEntry, error) {
	return []SkillEntry{}, nil
}

type demoRealtime struct{}

func newDemoRealtime() *demoRealtime { return &demoRealtime{} }

func (d *demoRealtime) Sessions(_ context.Context) ([]RealtimeSessionInfo, error) {
	return []RealtimeSessionInfo{}, nil
}

func (d *demoRealtime) Events(_ context.Context) ([]RealtimeEventInfo, error) {
	return []RealtimeEventInfo{}, nil
}
