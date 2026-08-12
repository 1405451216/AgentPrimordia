package dag

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// ===== VersionedWorkflow 测试 =====

func TestVersionedWorkflow_Create(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-wf")
	if err != nil {
		t.Fatalf("NewVersionedWorkflow 失败: %v", err)
	}
	if vw.ID != "test-wf" {
		t.Fatalf("ID = %q, want %q", vw.ID, "test-wf")
	}
	if vw.VersionCount() != 0 {
		t.Fatalf("初始版本数应为 0，实际为 %d", vw.VersionCount())
	}
}

func TestVersionedWorkflow_CreateEmptyID(t *testing.T) {
	_, err := NewVersionedWorkflow("")
	if err != ErrWorkflowIDEmpty {
		t.Fatalf("期望 ErrWorkflowIDEmpty，实际: %v", err)
	}
}

func TestVersionedWorkflow_NewVersion(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-wf")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	def := NewDAGWorkflow()
	if err := vw.NewVersion(def, "v1 description"); err != nil {
		t.Fatalf("NewVersion 失败: %v", err)
	}

	// 自动成为活跃版本
	if vw.ActiveVersion() == "" {
		t.Fatal("第一个版本应自动成为活跃版本")
	}

	versions := vw.ListVersions()
	if len(versions) != 1 {
		t.Fatalf("版本数 = %d, want 1", len(versions))
	}
	if versions[0].Description != "v1 description" {
		t.Fatalf("描述 = %q, want %q", versions[0].Description, "v1 description")
	}
}

func TestVersionedWorkflow_NewVersionWithID(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-wf")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	def := NewDAGWorkflow()
	if err := vw.NewVersionWithID("1.0.0", def, "first"); err != nil {
		t.Fatalf("NewVersionWithID 失败: %v", err)
	}

	// 重复版本号
	err = vw.NewVersionWithID("1.0.0", def, "duplicate")
	if err == nil {
		t.Fatal("重复版本号应返回错误")
	}

	// 空版本号
	err = vw.NewVersionWithID("", def, "empty")
	if err != ErrVersionEmpty {
		t.Fatalf("期望 ErrVersionEmpty，实际: %v", err)
	}
}

func TestVersionedWorkflow_GetActive(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-wf")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 无活跃版本时
	_, err = vw.GetActive()
	if err != ErrNoActiveVersion {
		t.Fatalf("期望 ErrNoActiveVersion，实际: %v", err)
	}

	// 添加版本后
	def := NewDAGWorkflow()
	if err := vw.NewVersionWithID("1.0.0", def, "first"); err != nil {
		t.Fatalf("NewVersionWithID 失败: %v", err)
	}

	active, err := vw.GetActive()
	if err != nil {
		t.Fatalf("GetActive 失败: %v", err)
	}
	if active.Version != "1.0.0" {
		t.Fatalf("活跃版本 = %q, want %q", active.Version, "1.0.0")
	}
}

func TestVersionedWorkflow_SetActive(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-wf")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	def := NewDAGWorkflow()
	_ = vw.NewVersionWithID("1.0.0", def, "first")
	_ = vw.NewVersionWithID("2.0.0", def, "second")

	// 切换到 2.0.0
	if err := vw.SetActive("2.0.0"); err != nil {
		t.Fatalf("SetActive 失败: %v", err)
	}
	if vw.ActiveVersion() != "2.0.0" {
		t.Fatalf("活跃版本 = %q, want %q", vw.ActiveVersion(), "2.0.0")
	}

	// 切换到不存在的版本
	err = vw.SetActive("9.9.9")
	if err == nil {
		t.Fatal("切换到不存在的版本应返回错误")
	}
}

func TestVersionedWorkflow_ListVersions(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-wf")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	def := NewDAGWorkflow()
	_ = vw.NewVersionWithID("1.0.0", def, "first")
	_ = vw.NewVersionWithID("2.0.0", def, "second")
	_ = vw.NewVersionWithID("3.0.0", def, "third")

	versions := vw.ListVersions()
	if len(versions) != 3 {
		t.Fatalf("版本数 = %d, want 3", len(versions))
	}

	// 验证顺序
	expected := []string{"1.0.0", "2.0.0", "3.0.0"}
	for i, v := range versions {
		if v.Version != expected[i] {
			t.Fatalf("versions[%d].Version = %q, want %q", i, v.Version, expected[i])
		}
	}
}

func TestVersionedWorkflow_RemoveVersion(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-wf")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	def := NewDAGWorkflow()
	_ = vw.NewVersionWithID("1.0.0", def, "first")
	_ = vw.NewVersionWithID("2.0.0", def, "second")

	// 不能移除活跃版本
	err = vw.RemoveVersion("1.0.0")
	if err == nil {
		t.Fatal("移除活跃版本应返回错误")
	}

	// 切换活跃版本后再移除
	_ = vw.SetActive("2.0.0")
	err = vw.RemoveVersion("1.0.0")
	if err != nil {
		t.Fatalf("移除非活跃版本失败: %v", err)
	}

	if vw.VersionCount() != 1 {
		t.Fatalf("版本数 = %d, want 1", vw.VersionCount())
	}
}

func TestVersionedWorkflow_GetVersion(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-wf")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	def := NewDAGWorkflow()
	_ = vw.NewVersionWithID("1.0.0", def, "first")

	v, err := vw.GetVersion("1.0.0")
	if err != nil {
		t.Fatalf("GetVersion 失败: %v", err)
	}
	if v.Version != "1.0.0" {
		t.Fatalf("Version = %q, want %q", v.Version, "1.0.0")
	}

	_, err = vw.GetVersion("9.9.9")
	if err == nil {
		t.Fatal("获取不存在的版本应返回错误")
	}
}

func TestVersionedWorkflow_ConcurrentAccess(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-wf")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	def := NewDAGWorkflow()
	_ = vw.NewVersionWithID("1.0.0", def, "first")

	// 并发读取和写入
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			_ = vw.NewVersionWithID(fmt.Sprintf("2.%d.0", i), def, "concurrent")
			_, _ = vw.GetActive()
			_ = vw.ListVersions()
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// 最终版本数应为 11 (1 + 10)
	if vw.VersionCount() != 11 {
		t.Fatalf("版本数 = %d, want 11", vw.VersionCount())
	}
}

// ===== HotMigration 测试 =====

func TestHotMigration_BasicPlan(t *testing.T) {
	from := NewDAGWorkflow()
	_ = from.AddNode(&DAGNode{ID: "a"})
	_ = from.AddNode(&DAGNode{ID: "b"})
	_ = from.AddEdge(DAGEdge{From: "a", To: "b"})

	to := NewDAGWorkflow()
	_ = to.AddNode(&DAGNode{ID: "a"})
	_ = to.AddNode(&DAGNode{ID: "b"})
	_ = to.AddNode(&DAGNode{ID: "c"})
	_ = to.AddEdge(DAGEdge{From: "a", To: "b"})
	_ = to.AddEdge(DAGEdge{From: "b", To: "c"})
	_ = to.AddEdge(DAGEdge{From: "a", To: "c"})

	plan := buildMigrationPlan(from, to)

	// a 和 b 是保留节点
	if len(plan.KeepNodes) != 2 {
		t.Fatalf("KeepNodes = %d, want 2", len(plan.KeepNodes))
	}
	// c 是新增节点
	if len(plan.AddNodes) != 1 {
		t.Fatalf("AddNodes = %d, want 1", len(plan.AddNodes))
	}
	if plan.AddNodes[0] != "c" {
		t.Fatalf("AddNodes[0] = %q, want %q", plan.AddNodes[0], "c")
	}
	// 无删除节点
	if len(plan.RemoveNodes) != 0 {
		t.Fatalf("RemoveNodes = %d, want 0", len(plan.RemoveNodes))
	}
}

func TestHotMigration_PlanWithRemovals(t *testing.T) {
	from := NewDAGWorkflow()
	_ = from.AddNode(&DAGNode{ID: "a"})
	_ = from.AddNode(&DAGNode{ID: "b"})
	_ = from.AddNode(&DAGNode{ID: "c"})

	to := NewDAGWorkflow()
	_ = to.AddNode(&DAGNode{ID: "a"})
	_ = to.AddNode(&DAGNode{ID: "d"})

	plan := buildMigrationPlan(from, to)

	// a 保留
	if len(plan.KeepNodes) != 1 {
		t.Fatalf("KeepNodes = %d, want 1", len(plan.KeepNodes))
	}
	// d 新增
	if len(plan.AddNodes) != 1 {
		t.Fatalf("AddNodes = %d, want 1", len(plan.AddNodes))
	}
	// b, c 删除
	if len(plan.RemoveNodes) != 2 {
		t.Fatalf("RemoveNodes = %d, want 2", len(plan.RemoveNodes))
	}
}

func TestHotMigration_ExecuteKeepRunning(t *testing.T) {
	from := NewDAGWorkflow()
	_ = from.AddNode(&DAGNode{ID: "a"})
	_ = from.AddNode(&DAGNode{ID: "b"})

	to := NewDAGWorkflow()
	_ = to.AddNode(&DAGNode{ID: "a"})
	_ = to.AddNode(&DAGNode{ID: "b"})
	_ = to.AddNode(&DAGNode{ID: "c"})

	plan := buildMigrationPlan(from, to)

	hm := &HotMigration{
		FromVersion:  "1.0.0",
		ToVersion:    "2.0.0",
		Strategy:     HotMigrationKeepRunning,
		Plan:         plan,
		FromWorkflow: from,
		ToWorkflow:   to,
	}

	ctx := context.Background()
	// v6.x：Execute 现在返回 ErrHotMigrationNotImplemented（plan generator
	// 仍生成 records，但不在飞 Run 上真正切换版本）。
	err := hm.Execute(ctx)
	if !errors.Is(err, ErrHotMigrationNotImplemented) {
		t.Fatalf("Execute 必须返回 ErrHotMigrationNotImplemented，实际: %v", err)
	}

	records := hm.Records()
	if len(records) != 3 {
		t.Fatalf("Records = %d, want 3", len(records))
	}

	// 已迁移节点包含新增的 c
	migrated := hm.MigratedNodeIDs()
	if len(migrated) != 1 || migrated[0] != "c" {
		t.Fatalf("MigratedNodeIDs = %v, want [c]", migrated)
	}
}

func TestHotMigration_ExecuteRestartAll(t *testing.T) {
	from := NewDAGWorkflow()
	_ = from.AddNode(&DAGNode{ID: "a"})
	_ = from.AddNode(&DAGNode{ID: "b"})

	to := NewDAGWorkflow()
	_ = to.AddNode(&DAGNode{ID: "a"})
	_ = to.AddNode(&DAGNode{ID: "b"})
	_ = to.AddNode(&DAGNode{ID: "c"})

	plan := buildMigrationPlan(from, to)

	hm := &HotMigration{
		FromVersion:  "1.0.0",
		ToVersion:    "2.0.0",
		Strategy:     HotMigrationRestartAll,
		Plan:         plan,
		FromWorkflow: from,
		ToWorkflow:   to,
	}

	ctx := context.Background()
	// v6.x：Execute 现在返回 ErrHotMigrationNotImplemented（plan generator
	// 仍生成 records，但不在飞 Run 上真正切换版本）。
	err := hm.Execute(ctx)
	if !errors.Is(err, ErrHotMigrationNotImplemented) {
		t.Fatalf("Execute 必须返回 ErrHotMigrationNotImplemented，实际: %v", err)
	}

	records := hm.Records()
	if len(records) != 3 {
		t.Fatalf("Records = %d, want 3", len(records))
	}

	// restart_all 策略下所有保留节点都标记为需要迁移
	migrated := hm.MigratedNodeIDs()
	if len(migrated) != 3 {
		t.Fatalf("MigratedNodeIDs = %d, want 3 (all nodes restarted)", len(migrated))
	}
}

func TestHotMigration_ExecuteGradual(t *testing.T) {
	from := NewDAGWorkflow()
	_ = from.AddNode(&DAGNode{ID: "a"})
	_ = from.AddNode(&DAGNode{ID: "b"})

	to := NewDAGWorkflow()
	_ = to.AddNode(&DAGNode{ID: "a"})
	_ = to.AddNode(&DAGNode{ID: "b"})
	_ = to.AddNode(&DAGNode{ID: "c"})

	plan := buildMigrationPlan(from, to)

	hm := &HotMigration{
		FromVersion:  "1.0.0",
		ToVersion:    "2.0.0",
		Strategy:     HotMigrationGradual,
		Plan:         plan,
		FromWorkflow: from,
		ToWorkflow:   to,
	}

	ctx := context.Background()
	// v6.x：Execute 现在返回 ErrHotMigrationNotImplemented（plan generator
	// 仍生成 records，但不在飞 Run 上真正切换版本）。
	err := hm.Execute(ctx)
	if !errors.Is(err, ErrHotMigrationNotImplemented) {
		t.Fatalf("Execute 必须返回 ErrHotMigrationNotImplemented，实际: %v", err)
	}

	records := hm.Records()
	if len(records) != 3 {
		t.Fatalf("Records = %d, want 3", len(records))
	}
}

func TestHotMigration_ExecuteUnknownStrategy(t *testing.T) {
	hm := &HotMigration{
		Strategy: "invalid_strategy",
		Plan:     &MigrationPlan{Mapping: map[string]string{}},
	}
	err := hm.Execute(context.Background())
	if err == nil {
		t.Fatal("未知策略应返回错误")
	}
}

func TestHotMigration_ExecuteNilPlan(t *testing.T) {
	hm := &HotMigration{
		Strategy: HotMigrationKeepRunning,
		Plan:     nil,
	}
	err := hm.Execute(context.Background())
	if err == nil {
		t.Fatal("nil Plan 应返回错误")
	}
}

func TestVersionedWorkflow_Migrate(t *testing.T) {
	vw, err := NewVersionedWorkflow("test-migration")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 版本 1.0.0
	v1 := NewDAGWorkflow()
	_ = v1.AddNode(&DAGNode{ID: "a"})
	_ = v1.AddNode(&DAGNode{ID: "b"})
	_ = v1.AddEdge(DAGEdge{From: "a", To: "b"})
	if err := vw.NewVersionWithID("1.0.0", v1, "first version"); err != nil {
		t.Fatalf("NewVersionWithID 失败: %v", err)
	}

	// 版本 2.0.0（新增节点 c）
	v2 := NewDAGWorkflow()
	_ = v2.AddNode(&DAGNode{ID: "a"})
	_ = v2.AddNode(&DAGNode{ID: "b"})
	_ = v2.AddNode(&DAGNode{ID: "c"})
	_ = v2.AddEdge(DAGEdge{From: "a", To: "b"})
	_ = v2.AddEdge(DAGEdge{From: "b", To: "c"})
	if err := vw.NewVersionWithID("2.0.0", v2, "second version"); err != nil {
		t.Fatalf("NewVersionWithID 失败: %v", err)
	}

	// 执行迁移
	ctx := context.Background()
	// v6.x：Migrate 现在透传 ErrHotMigrationNotImplemented，但仍把 plan 写入 records。
	err = vw.Migrate(ctx, "1.0.0", "2.0.0")
	if !errors.Is(err, ErrHotMigrationNotImplemented) {
		t.Fatalf("Migrate 必须返回 ErrHotMigrationNotImplemented，实际: %v", err)
	}

	// 验证活跃版本已切换
	if vw.ActiveVersion() != "2.0.0" {
		t.Fatalf("活跃版本 = %q, want %q", vw.ActiveVersion(), "2.0.0")
	}
}

func TestVersionedWorkflow_MigrateSameVersion(t *testing.T) {
	vw, err := NewVersionedWorkflow("test")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	def := NewDAGWorkflow()
	_ = vw.NewVersionWithID("1.0.0", def, "first")

	err = vw.Migrate(context.Background(), "1.0.0", "1.0.0")
	if err != ErrMigrationSameVersion {
		t.Fatalf("期望 ErrMigrationSameVersion，实际: %v", err)
	}
}

func TestVersionedWorkflow_MigrateSourceNotFound(t *testing.T) {
	vw, err := NewVersionedWorkflow("test")
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	def := NewDAGWorkflow()
	_ = vw.NewVersionWithID("2.0.0", def, "second")

	err = vw.Migrate(context.Background(), "1.0.0", "2.0.0")
	if err == nil {
		t.Fatal("源版本不存在应返回错误")
	}
}

func TestHotMigration_NodeDiff(t *testing.T) {
	// 测试节点结构差异检测
	a := &DAGNode{
		ID:    "node1",
		Input: "input1",
		Metadata: map[string]string{
			"label": "Node 1",
		},
	}
	b := &DAGNode{
		ID:    "node1",
		Input: "input1",
		Metadata: map[string]string{
			"label": "Node 1",
		},
	}
	if nodesDiffer(a, b) {
		t.Fatal("相同节点不应被判别为不同")
	}

	// Input 不同
	c := &DAGNode{
		ID:    "node1",
		Input: "input2",
	}
	if !nodesDiffer(a, c) {
		t.Fatal("Input 不同应判别为不同")
	}

	// Metadata 不同
	d := &DAGNode{
		ID:    "node1",
		Input: "input1",
		Metadata: map[string]string{
			"label": "Node X",
		},
	}
	if !nodesDiffer(a, d) {
		t.Fatal("Metadata 不同应判别为不同")
	}

	// RetryPolicy 不同
	e := &DAGNode{
		ID:          "node1",
		Input:       "input1",
		RetryPolicy: &RetryPolicy{MaxRetries: 3},
	}
	if !nodesDiffer(a, e) {
		t.Fatal("RetryPolicy 不同应判别为不同")
	}
}

func TestHotMigration_PlanHelpers(t *testing.T) {
	plan := &MigrationPlan{
		KeepNodes:   []string{"a", "b"},
		AddNodes:    []string{"c"},
		RemoveNodes: []string{"d"},
		ModifyNodes: []string{"b"},
		Mapping:     map[string]string{"a": "a", "b": "b"},
	}

	if !plan.IsAdded("c") {
		t.Fatal("c 应为新增节点")
	}
	if plan.IsAdded("a") {
		t.Fatal("a 不应为新增节点")
	}
	if !plan.IsRemoved("d") {
		t.Fatal("d 应为删除节点")
	}
	if plan.IsRemoved("a") {
		t.Fatal("a 不应为删除节点")
	}
	if !plan.HasNodeChange("b") {
		t.Fatal("b 应有结构变更")
	}
	if plan.HasNodeChange("a") {
		t.Fatal("a 不应有结构变更")
	}
}
