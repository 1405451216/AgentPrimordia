// fixtures_test.go — 世界模型跨语言对账夹具（Go 为权威生成方）
//
// 夹具文件 testdata/worldmodel_fixtures.json 覆盖：
//   - NodeID（FNV-1a64 确定性 ID 派生：ASCII / 空白规范化 / 中文 / Kind 区分）
//   - ComparePaths（一致 / 乱序 / 计划外 / 跳步四种差异形态）
//   - Rehearse（通过 / 前提缺失，含中文缺陷文案双线一致）
//   - Snapshot（JSON 键名即 Go 序列化形态，TS Restore 直接消费同一 JSON）
//
// TS 侧对账门：sdk/typescript/src/agent/__tests__/worldmodel.test.ts。
// 再生方式：AP_WRITE_WORLD_FIXTURE=1 go test ./internal/agent/worldmodel/ -run TestWriteWorldModelFixtures
package worldmodel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type fxNodeID struct {
	Kind    NodeKind `json:"kind"`
	Summary string   `json:"summary"`
	ID      string   `json:"id"`
}

type fxCompare struct {
	Planned []string `json:"planned"`
	Actual  []string `json:"actual"`
	Diff    BackDiff `json:"diff"`
}

type fxRehearse struct {
	Plan       Plan        `json:"plan"`
	GraphNodes []StateNode `json:"graph_nodes"`
	Pass       bool        `json:"pass"`
	Missing    []string    `json:"missing"`
}

type wmFixtures struct {
	NodeID       []fxNodeID   `json:"nodeID"`
	ComparePaths []fxCompare  `json:"comparePaths"`
	Rehearse     []fxRehearse `json:"rehearse"`
	Snapshot     Snapshot     `json:"snapshot"`
}

func buildFixtures(t *testing.T) wmFixtures {
	t.Helper()
	f := wmFixtures{}

	// NodeID 夹具：ASCII / 空白规范化 / 中文 / Kind 区分
	for _, c := range []struct {
		kind    NodeKind
		summary string
	}{
		{KindToolCall, `read_file {"path":"a.txt"}`},
		{KindToolCall, "  read_file \t {\"path\":\"a.txt\"} "},
		{KindObservation, "观察：文件已写入"},
		{KindObservation, `read_file {"path":"a.txt"}`},
		{KindHypothesis, "假设：缓存命中导致结果一致"},
	} {
		f.NodeID = append(f.NodeID, fxNodeID{Kind: c.kind, Summary: c.summary, ID: NodeID(c.kind, c.summary)})
	}

	// ComparePaths 夹具：一致 / 乱序 / 计划外追加 / 跳步
	for _, c := range []struct{ planned, actual []string }{
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{[]string{"a", "b", "c"}, []string{"a", "c", "b"}},
		{[]string{"a"}, []string{"a", "x"}},
		{[]string{"a", "b", "c"}, []string{"a"}},
	} {
		f.ComparePaths = append(f.ComparePaths, fxCompare{
			Planned: c.planned, Actual: c.actual,
			Diff: ComparePaths(c.planned, c.actual),
		})
	}

	// Rehearse 夹具：图上既有节点 + 计划依赖（真实调用 Rehearse 采集判定与文案）
	g := NewStateGraph()
	existingID, _ := g.AddNode(KindObservation, "图上既有节点", 0)
	planPass := Plan{Goal: "通过计划", Steps: []PlanStep{
		{ID: "s1", Summary: "第一步", DependsOn: []string{existingID}},
	}}
	repPass := Rehearse(planPass, g)
	f.Rehearse = append(f.Rehearse, fxRehearse{
		Plan:       planPass,
		GraphNodes: g.Nodes(),
		Pass:       repPass.Pass,
		Missing:    repPass.MissingPreconditions,
	})
	planDefect := Plan{Goal: "缺陷计划", Steps: []PlanStep{
		{ID: "s1", Summary: "第一步", DependsOn: []string{"幽灵节点"}},
		{ID: "s1", Summary: "第一步重复 ID"},
		{ID: "s2", Summary: "自依赖", DependsOn: []string{"s2"}},
		{ID: "s3", Summary: "前向依赖", DependsOn: []string{"s4"}},
		{ID: "s4", Summary: "第四步"},
	}}
	repDefect := Rehearse(planDefect, nil)
	f.Rehearse = append(f.Rehearse, fxRehearse{
		Plan:       planDefect,
		GraphNodes: nil,
		Pass:       repDefect.Pass,
		Missing:    repDefect.MissingPreconditions,
	})

	// Snapshot 夹具：任务 + 计划 + 因果链 + 被裁事实 + 轨迹
	tr := NewWorldModelTracker()
	tr.Apply(PlanRevised{Turn: 1, Task: "整理任务", Goal: "整理", Steps: []PlanStep{
		{ID: NodeID(KindToolCall, "read a"), Summary: "read a"},
	}})
	tr.Apply(ToolObserved{Turn: 1, ToolName: "read", ToolInput: "a", Observation: "内容A"})
	tr.Apply(HypothesisFormed{Turn: 1, Text: "假设：文件存在"})
	tr.TrimNotification([]TrimmedMessage{{Role: "user", Content: "被裁历史"}}, 2)
	f.Snapshot = tr.Snapshot()
	return f
}

// TestWriteWorldModelFixtures 生成夹具（默认跳过；AP_WRITE_WORLD_FIXTURE=1 时写出）
func TestWriteWorldModelFixtures(t *testing.T) {
	if os.Getenv("AP_WRITE_WORLD_FIXTURE") == "" {
		t.Skip("设置 AP_WRITE_WORLD_FIXTURE=1 以重新生成夹具")
	}
	f := buildFixtures(t)
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "worldmodel_fixtures.json"), append(data, byte(10)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log("夹具已写出")
}

// TestWorldModelFixturesGolden 夹具黄金门：testdata/worldmodel_fixtures.json
// 必须与当前实现逐位一致——任何确定性语义漂移（ID 派生/差异判定/文案/快照
// 形态）即测试失败，TS 侧对账以同一文件为基准。
func TestWorldModelFixturesGolden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "worldmodel_fixtures.json"))
	if err != nil {
		t.Skipf("夹具文件不存在（先以 AP_WRITE_WORLD_FIXTURE=1 生成）: %v", err)
	}
	var golden wmFixtures
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatalf("夹具解析失败: %v", err)
	}
	current, err := json.Marshal(buildFixtures(t))
	if err != nil {
		t.Fatal(err)
	}
	var currentFixtures wmFixtures
	if err := json.Unmarshal(current, &currentFixtures); err != nil {
		t.Fatal(err)
	}
	g1, _ := json.Marshal(golden)
	g2, _ := json.Marshal(currentFixtures)
	if string(g1) != string(g2) {
		t.Fatal("世界模型确定性语义漂移：夹具与当前实现不一致（AP_WRITE_WORLD_FIXTURE=1 重新生成前先评审）")
	}
}
