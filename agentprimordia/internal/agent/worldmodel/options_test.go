// options_test.go — WithWorldModel 选项骨架测试
package worldmodel

import "testing"

func TestWithWorldModel(t *testing.T) {
	t.Run("绑定 tracker", func(t *testing.T) {
		tr := NewWorldModelTracker()
		o := NewWorldModelOptions(WithWorldModel(tr))
		if o.Tracker != tr {
			t.Errorf("WithWorldModel 应把 tracker 绑定进选项：got %v", o.Tracker)
		}
	})

	t.Run("允许 nil tracker（等价不启用）", func(t *testing.T) {
		o := NewWorldModelOptions(WithWorldModel(nil))
		if o.Tracker != nil {
			t.Errorf("nil tracker 应原样绑定（调用方自行短路）：got %v", o.Tracker)
		}
	})

	t.Run("无选项时零值即默认关闭", func(t *testing.T) {
		o := NewWorldModelOptions()
		if o.Tracker != nil {
			t.Errorf("默认应不启用世界模型（Tracker==nil）：got %v", o.Tracker)
		}
	})

	t.Run("nil 选项跳过；多选项后者覆盖", func(t *testing.T) {
		tr1 := NewWorldModelTracker()
		tr2 := NewWorldModelTracker()
		o := NewWorldModelOptions(nil, WithWorldModel(tr1), nil, WithWorldModel(tr2))
		if o.Tracker != tr2 {
			t.Errorf("多选项应后者覆盖：got %v want %v", o.Tracker, tr2)
		}
	})
}
