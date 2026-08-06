package skills

import "fmt"

// Validator 技能规范校验器
type Validator struct{}

// NewValidator 创建校验器
func NewValidator() *Validator {
	return &Validator{}
}

// Validate 校验技能规范合法性
func (v *Validator) Validate(s *Skill) error {
	if s.Name == "" {
		return fmt.Errorf("skills: 技能名称不能为空")
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("skills: 技能 %s 至少需要一个步骤", s.Name)
	}

	// 步骤 ID 唯一性
	stepIDs := make(map[string]bool, len(s.Steps))
	for _, step := range s.Steps {
		if step.ID == "" {
			return fmt.Errorf("skills: 技能 %s 存在空步骤 ID", s.Name)
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("skills: 技能 %s 步骤 ID 重复: %s", s.Name, step.ID)
		}
		stepIDs[step.ID] = true
	}

	// 依赖引用有效性 + 循环检测
	for _, step := range s.Steps {
		for _, dep := range step.DependsOn {
			if !stepIDs[dep] {
				return fmt.Errorf("skills: 步骤 %s 依赖不存在的步骤 %s", step.ID, dep)
			}
		}
	}

	if err := v.detectCycle(s.Steps); err != nil {
		return err
	}

	// 必填输入字段校验
	for _, req := range s.Input.Required {
		if _, ok := s.Input.Fields[req]; !ok {
			return fmt.Errorf("skills: 必填输入字段 %s 未在 schema 中定义", req)
		}
	}

	return nil
}

// detectCycle 检测步骤依赖环
func (v *Validator) detectCycle(steps []StepDef) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(steps))
	adj := make(map[string][]string, len(steps))
	for _, s := range steps {
		adj[s.ID] = s.DependsOn
	}

	var dfs func(id string) bool
	dfs = func(id string) bool {
		color[id] = gray
		for _, dep := range adj[id] {
			if color[dep] == gray {
				return true
			}
			if color[dep] == white && dfs(dep) {
				return true
			}
		}
		color[id] = black
		return false
	}

	for _, s := range steps {
		if color[s.ID] == white {
			if dfs(s.ID) {
				return fmt.Errorf("skills: 技能步骤存在循环依赖")
			}
		}
	}
	return nil
}

// SecurityScan 安全扫描（防恶意技能）
func (v *Validator) SecurityScan(s *Skill) []string {
	var warnings []string
	for _, step := range s.Steps {
		// 检测危险工具调用
		dangerousTools := map[string]bool{
			"shell_exec": true, "rm_rf": true, "system": true,
		}
		if dangerousTools[step.ToolName] {
			warnings = append(warnings, fmt.Sprintf("步骤 %s 调用高风险工具 %s", step.ID, step.ToolName))
		}
	}
	return warnings
}
