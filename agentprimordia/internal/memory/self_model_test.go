// self_model_test.go — v5.3 自我模型记忆测试
package memory

import (
	"strings"
	"testing"
)

func TestSelfModelRecordAndWeakDomains(t *testing.T) {
	m := NewSelfModel()
	// go_testing 强项：9 成
	for i := 0; i < 9; i++ {
		m.RecordOutcome("go_testing", true, 3, "")
	}
	m.RecordOutcome("go_testing", false, 8, "timeout")
	// sql_migration 弱项：2 成
	for i := 0; i < 8; i++ {
		m.RecordOutcome("sql_migration", false, 12, "lock_timeout")
	}
	m.RecordOutcome("sql_migration", true, 5, "")

	weak := m.WeakDomains(3, 50)
	if len(weak) != 1 || weak[0].Domain != "sql_migration" {
		t.Fatalf("弱项应为 sql_migration，得到 %+v", weak)
	}

	top := m.TopFailures(1)
	if len(top) != 1 || top[0].Signature != "lock_timeout" || top[0].Count != 8 {
		t.Errorf("高频失败签名错误: %+v", top)
	}
}

func TestSelfModelMitigationAndPromptInjection(t *testing.T) {
	m := NewSelfModel()
	for i := 0; i < 5; i++ {
		m.RecordOutcome("deploy", false, 6, "image_pull_backoff")
	}
	m.SetMitigation("image_pull_backoff", "预先 pull 镜像并延长超时")

	prompt := m.InjectIntoSystemPrompt("你是运维 Agent。")
	if !strings.Contains(prompt, "自我能力画像") {
		t.Error("注入后应包含画像段")
	}
	if !strings.Contains(prompt, "image_pull_backoff") || !strings.Contains(prompt, "预先 pull") {
		t.Error("应包含失败签名与缓解手段")
	}
	if !strings.Contains(prompt, "你是运维 Agent。") {
		t.Error("基础提示词应保留在开头")
	}

	// 空模型不注入
	empty := NewSelfModel().InjectIntoSystemPrompt("BASE")
	if empty != "BASE" {
		t.Errorf("空画像不应修改提示词: %q", empty)
	}
}

func TestSelfModelMarshalRoundTrip(t *testing.T) {
	m := NewSelfModel()
	m.RecordOutcome("a", true, 2, "")
	m.RecordOutcome("b", false, 4, "sig-x")

	data, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || !strings.Contains(string(data), "sig-x") {
		t.Error("导出应包含失败签名")
	}
}
