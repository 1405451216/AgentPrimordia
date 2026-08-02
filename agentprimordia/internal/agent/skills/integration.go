package skills

// v3.4 Phase 2: 技能 × 跨组件集成接口
//
// 所有集成通过接口解耦，运行时可选注入，未配置时自动跳过。

// --- 技能 × 工具 ---

// ToolInvoker 工具调用接口（由外部 tools/ 注册表适配，含 Scope 权限校验 + MCP）
type ToolInvoker interface {
	// Invoke 调用指定工具
	Invoke(toolName string, params map[string]any) (string, error)
	// HasTool 检查工具是否注册
	HasTool(toolName string) bool
}

// ToolIntegration 技能执行时调用工具注册表
type ToolIntegration struct {
	invoker ToolInvoker
}

// NewToolIntegration 创建工具集成
func NewToolIntegration(invoker ToolInvoker) *ToolIntegration {
	return &ToolIntegration{invoker: invoker}
}

// InvokeStepTool 执行技能步骤对应的工具（含存在性校验）
func (t *ToolIntegration) InvokeStepTool(step StepDef, params map[string]any) (string, error) {
	if !t.invoker.HasTool(step.ToolName) {
		return "", &SkillError{Msg: "技能步骤工具未注册: " + step.ToolName}
	}
	return t.invoker.Invoke(step.ToolName, params)
}

// --- 技能 × 学习 ---

// KnowledgeProvider 知识提供接口（由外部 learning/ 蒸馏适配）
type KnowledgeProvider interface {
	// GetKnowledge 获取与技能相关的蒸馏知识，作为技能构建块
	GetKnowledge(topic string) ([]string, error)
}

// LearningIntegration 蒸馏知识 → 技能构建块
type LearningIntegration struct {
	provider KnowledgeProvider
}

// NewLearningIntegration 创建学习集成
func NewLearningIntegration(provider KnowledgeProvider) *LearningIntegration {
	return &LearningIntegration{provider: provider}
}

// EnrichSkillDescription 用蒸馏知识增强技能描述
func (l *LearningIntegration) EnrichSkillDescription(skill *Skill, topic string) error {
	items, err := l.provider.GetKnowledge(topic)
	if err != nil {
		return err
	}
	if len(items) > 0 {
		skill.Metadata["knowledge_source"] = topic
		skill.Metadata["knowledge_count"] = itoa(len(items))
	}
	return nil
}

// --- 技能 × 市场 ---

// MarketplacePublisher 市场发布接口（由外部 marketplace/ 适配，能力级共享）
type MarketplacePublisher interface {
	// Publish 发布技能到能力级市场
	Publish(skill *Skill) (string, error)
	// Unpublish 从市场下架
	Unpublish(skillID string) error
}

// MarketplaceIntegration 技能发布到市场
type MarketplaceIntegration struct {
	publisher MarketplacePublisher
}

// NewMarketplaceIntegration 创建市场集成
func NewMarketplaceIntegration(publisher MarketplacePublisher) *MarketplaceIntegration {
	return &MarketplaceIntegration{publisher: publisher}
}

// PublishSkill 发布已验证技能到市场
func (m *MarketplaceIntegration) PublishSkill(skill *Skill) (string, error) {
	if skill.Status != SkillActive && skill.Status != SkillVerified {
		return "", &SkillError{Msg: "仅 verified/active 技能可发布，当前: " + string(skill.Status)}
	}
	return m.publisher.Publish(skill)
}

// --- 技能 × 自治 ---

// AutonomySkillHook 自治执行钩子（v3.3 联动：自治目标执行中习得技能）
type AutonomySkillHook interface {
	// OnGoalComplete 目标完成时回调，触发技能习得评估
	OnGoalComplete(goalID string, success bool, taskType string)
	// SuggestSkill 为目标推荐已有技能（返回 skillID，空表示无）
	SuggestSkill(goalDescription string) string
}

// AutonomyIntegration 自治 × 技能联动
type AutonomyIntegration struct {
	hook    AutonomySkillHook
	matcher *Matcher
}

// NewAutonomyIntegration 创建自治集成
func NewAutonomyIntegration(hook AutonomySkillHook, matcher *Matcher) *AutonomyIntegration {
	return &AutonomyIntegration{hook: hook, matcher: matcher}
}

// SuggestForGoal 为自治目标推荐技能
func (a *AutonomyIntegration) SuggestForGoal(goalDescription string) string {
	if a.matcher != nil {
		if r := a.matcher.Match(goalDescription); r != nil {
			return r.Skill.ID
		}
	}
	if a.hook != nil {
		return a.hook.SuggestSkill(goalDescription)
	}
	return ""
}

// --- 技能 × RAG ---

// RAGKnowledgeSink RAG 知识沉淀接口（习得验证的测试用例存入 RAG 作回归知识）
type RAGKnowledgeSink interface {
	// StoreTestCase 存储技能测试用例为 RAG 知识
	StoreTestCase(skillID string, tc TestCase) error
}

// RAGIntegration 技能测试用例 → RAG 回归知识
type RAGIntegration struct {
	sink RAGKnowledgeSink
}

// NewRAGIntegration 创建 RAG 集成
func NewRAGIntegration(sink RAGKnowledgeSink) *RAGIntegration {
	return &RAGIntegration{sink: sink}
}

// SinkTestCases 将验证用例沉淀到 RAG
func (r *RAGIntegration) SinkTestCases(skillID string, cases []TestCase) error {
	for _, tc := range cases {
		if err := r.sink.StoreTestCase(skillID, tc); err != nil {
			return err
		}
	}
	return nil
}

// SkillError 技能集成错误
type SkillError struct{ Msg string }

func (e *SkillError) Error() string { return "skills: " + e.Msg }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
