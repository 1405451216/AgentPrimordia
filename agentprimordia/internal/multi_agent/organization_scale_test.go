// organization_scale_test.go — v5.5 验收：组织规模扩展基准（延续 v3.8 口径）。
//
// 验收（V6-ROADMAP §七 任务 1）：组织规模 N 翻倍成功率下降 ≤2%（每组 N≥20 次重复）。
// 设计：成员带确定性领域胜任表——小组织覆盖不全的域必然失败；
// 翻倍后专家覆盖更全。涌现分工路由应使大组织成功率 ≥ 小组织 - 2%。
package multi_agent

import (
	"context"
	"fmt"
	"testing"
)

type scaleAgent struct {
	name   string
	goodAt map[string]bool // 胜任域集合
}

func (s *scaleAgent) Name() string { return s.name }
func (s *scaleAgent) Execute(_ context.Context, task string) (string, error) {
	domain := task
	if i := len(task); i > 0 {
		for j := 0; j < len(task); j++ {
			if task[j] == '|' {
				domain = task[:j]
				break
			}
		}
	}
	if s.goodAt[domain] {
		return "ok", nil
	}
	return "", fmt.Errorf("not competent in %s", domain)
}

func orgSuccessRate(t *testing.T, members int, reps int) float64 {
	t.Helper()
	org := NewOrganization()
	domains := []string{"d1", "d2", "d3", "d4", "d5", "d6", "d7", "d8"}
	for i := 0; i < members; i++ {
		good := map[string]bool{}
		// 每成员胜任 1 个域：4 人覆盖 4/8 域，8 人(翻倍)全覆盖——
		// 专家覆盖随规模扩大是组织规模收益的来源
		good[domains[i%len(domains)]] = true
		org.Register(&scaleAgent{name: fmt.Sprintf("m%d", i), goodAt: good})
	}

	successes := 0
	total := 0
	for rep := 0; rep < reps; rep++ {
		tasks := make([]string, len(domains))
		for di, d := range domains {
			tasks[di] = fmt.Sprintf("%s|任务%d-%d", d, rep, di)
		}
		report := org.Execute(context.Background(), tasks)
		successes += report.Completed
		total += report.Failed + report.Completed
	}
	return float64(successes) / float64(total)
}

func TestOrganizationScaleDoublingNoDegradation(t *testing.T) {
	const reps = 20 // 每组 20 次重复 × 8 任务
	small := orgSuccessRate(t, 4, reps)
	large := orgSuccessRate(t, 8, reps)
	t.Logf("规模 4: 成功率 %.3f | 规模 8(翻倍): 成功率 %.3f", small, large)
	if large < small-0.02 {
		t.Errorf("翻倍后成功率下降超阈值：%.3f → %.3f（允许降幅 2%%）", small, large)
	}
	if large <= small {
		t.Errorf("专家覆盖翻倍应带来提升或持平：%.3f → %.3f", small, large)
	}
}
