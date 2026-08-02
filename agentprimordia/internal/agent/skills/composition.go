package skills

import "fmt"

// Composition 技能组合：多技能编排为工作流
type Composition struct {
	// ID 组合标识
	ID string `json:"id"`
	// Name 组合名称
	Name string `json:"name"`
	// SkillRefs 引用的技能 ID（有序）
	SkillRefs []string `json:"skill_refs"`
	// DataFlow 步骤间数据流映射（上游技能输出键 → 下游技能输入键）
	DataFlow map[string]string `json:"data_flow,omitempty"`
}

// NewComposition 创建技能组合
func NewComposition(name string, skillRefs []string) *Composition {
	return &Composition{
		ID:        generateSkillID(),
		Name:      name,
		SkillRefs: skillRefs,
		DataFlow:  make(map[string]string),
	}
}

// Validate 校验组合合法性
func (c *Composition) Validate(store *Store) error {
	if len(c.SkillRefs) == 0 {
		return fmt.Errorf("skills: 组合 %s 至少需要引用一个技能", c.Name)
	}
	for _, ref := range c.SkillRefs {
		s, ok := store.Get(ref)
		if !ok {
			return fmt.Errorf("skills: 组合引用的技能 %s 不存在", ref)
		}
		if s.Status != SkillActive {
			return fmt.Errorf("skills: 组合引用的技能 %s 未激活（状态: %s）", ref, s.Status)
		}
	}
	return nil
}
