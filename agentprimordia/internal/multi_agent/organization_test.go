// organization_test.go — v5.5 组织智能测试：黑板 / 涌现分工 / 组织级调度
package multi_agent

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestBlackboardPostReadAndClaim(t *testing.T) {
	bb := NewBlackboard()
	bb.Post(Entry{TaskID: "t1", Author: "planner", Kind: KindDirective, Content: "实现登录接口"})
	bb.Post(Entry{TaskID: "t1", Author: "coder", Kind: KindClaim, Content: "认领"})
	bb.Post(Entry{TaskID: "t2", Author: "planner", Kind: KindDirective, Content: "编写文档"})

	t1 := bb.Read("t1")
	if len(t1) != 2 {
		t.Fatalf("t1 应有 2 条，得到 %d", len(t1))
	}
	if t1[0].Content != "实现登录接口" {
		t.Errorf("时间序错误: %+v", t1[0])
	}

	// 认领租约：第一个成功，重复被拒
	if !bb.Claim("t1", "agent-a") {
		t.Fatal("首次认领应成功")
	}
	if bb.Claim("t1", "agent-b") {
		t.Error("已被认领的任务不得重复认领")
	}
	if holder := bb.ClaimHolder("t1"); holder != "agent-a" {
		t.Errorf("持有人应为 agent-a，得到 %q", holder)
	}
	// 释放后可再认领
	bb.Release("t1")
	if !bb.Claim("t1", "agent-b") {
		t.Error("释放后应可认领")
	}
}

func TestOrgRouterLearnsSpecialization(t *testing.T) {
	r := NewOrgRouter()
	// agent-a 在 go_testing 连续成功；agent-b 连续失败
	for i := 0; i < 5; i++ {
		r.Record("agent-a", "go_testing", true)
		r.Record("agent-b", "go_testing", false)
	}
	if got := r.Route("go_testing", []string{"agent-a", "agent-b"}); got != "agent-a" {
		t.Errorf("应路由到历史更优的 agent-a，得到 %q", got)
	}
	// 冷启动域：回退到候选首位
	if got := r.Route("unknown_domain", []string{"agent-b", "agent-a"}); got != "agent-b" {
		t.Errorf("冷启动应按候选顺序回退，得到 %q", got)
	}
}

func TestOrgRouterConcurrentRecord(t *testing.T) {
	r := NewOrgRouter()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); r.Record("a", "d", i%2 == 0) }(i)
	}
	wg.Wait()
}

// stubOrgAgent 组织内成员 Agent
type stubOrgAgent struct{ name, reply string }

func (s *stubOrgAgent) Name() string { return s.name }
func (s *stubOrgAgent) Execute(_ context.Context, task string) (string, error) {
	return s.reply + ":" + task, nil
}

func TestOrganizationExecuteEndToEnd(t *testing.T) {
	org := NewOrganization()
	org.Register(&stubOrgAgent{name: "tester", reply: "测试完成"})
	org.Register(&stubOrgAgent{name: "coder", reply: "编码完成"})
	// 教组织：tester 擅长 test 类任务
	for i := 0; i < 4; i++ {
		org.Router.Record("tester", "test", true)
		org.Router.Record("coder", "test", false)
	}

	report := org.Execute(context.Background(), []string{
		"test|为登录模块写测试",
		"code|实现登录逻辑",
	})
	if report.Completed != 2 || report.Failed != 0 {
		t.Fatalf("完成度错误: %+v", report)
	}
	if len(report.Results) != 2 {
		t.Fatalf("结果数错误: %d", len(report.Results))
	}
	// 黑板应有完整轨迹（directive+claim+result × 2）
	if got := len(org.Board.Read("")); got < 6 {
		t.Errorf("黑板轨迹过少: %d", got)
	}
	// 分工学习验证：test 任务路由给 tester
	foundTesterOnTest := false
	for _, e := range org.Board.Read("") {
		if e.Kind == KindClaim && e.Author == "tester" && strings.Contains(e.Content, "test") {
			foundTesterOnTest = true
		}
	}
	if !foundTesterOnTest {
		t.Error("test 任务应由 tester 认领（涌现分工）")
	}
}
