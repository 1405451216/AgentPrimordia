// deprecated_check.go — v1.0.0 Deprecated 字段 panic 检查
//
// 按照废弃时间表（docs/specs/2026-06-04-semver-policy.md §4.2），
// v1.0.0 起 ReActConfig 的 14 个 Deprecated 字段如果被设置，NewReActAgent 将 panic。
// 强制用户迁移到 NewAgent + Functional Options 或链式 API。
package agent

import "fmt"

// migrationGuideURL 是迁移指南的位置
const migrationGuideURL = "ecosystem/docs/migration/v0-deprecations.md"

// checkDeprecatedFields 检查 ReActConfig 的 14 个 Deprecated 字段是否被设置。
// 如果任一字段非 nil/非零，panic 并给出迁移建议。
//
// 纯标量配置（Name/SystemPrompt/Model/MaxTurns/Temperature/SessionID/Lifecycle/Logger）
// 不会触发 panic，保持向后兼容。
func checkDeprecatedFields(cfg ReActConfig) {
	// 按字段定义顺序检查
	if cfg.Toolkit != nil {
		panic(deprecatedPanicMsg("Toolkit",
			"NewAgent(name, prompt, model, WithToolkit(registry))"))
	}
	if cfg.Memory != nil {
		panic(deprecatedPanicMsg("Memory",
			"NewAgent(name, prompt, model, WithMemory(store))"))
	}
	if cfg.EventPublisher != nil {
		panic(deprecatedPanicMsg("EventPublisher",
			"NewAgent(name, prompt, model, WithEvents(publisher))"))
	}
	if cfg.Metrics != nil {
		panic(deprecatedPanicMsg("Metrics",
			"NewAgent(name, prompt, model, WithMetrics(recorder))"))
	}
	if cfg.ContextWindow != nil {
		panic(deprecatedPanicMsg("ContextWindow",
			"NewAgent(name, prompt, model, WithContextWindow(strategy))"))
	}
	if cfg.CheckpointStore != nil {
		panic(deprecatedPanicMsg("CheckpointStore",
			"NewAgent(name, prompt, model, WithCheckpointStore(store))"))
	}
	if cfg.RAG != nil {
		panic(deprecatedPanicMsg("RAG",
			"NewAgent(name, prompt, model, WithRAG(ragConfig))"))
	}
	if cfg.Hooks != nil {
		panic(deprecatedPanicMsg("Hooks",
			"NewAgent(name, prompt, model, WithHooks(hookManager))"))
	}
	if cfg.Summarizer != nil {
		panic(deprecatedPanicMsg("Summarizer",
			"NewAgent(name, prompt, model, WithSummarizer(extractor))"))
	}
	if len(cfg.FileScope) > 0 {
		panic(deprecatedPanicMsg("FileScope",
			"NewAgent(name, prompt, model, WithFileScope(scopes))"))
	}
	if cfg.HITL != nil {
		panic(deprecatedPanicMsg("HITL",
			"NewAgent(name, prompt, model, WithHITL(hiltConfig))"))
	}
	if cfg.CostTracker != nil {
		panic(deprecatedPanicMsg("CostTracker",
			"NewAgent(name, prompt, model, WithCostTracker(tracker))"))
	}
	if cfg.Tracer != nil {
		panic(deprecatedPanicMsg("Tracer",
			"NewAgent(name, prompt, model, WithTracer(tracer))"))
	}
	if cfg.Cache != nil {
		panic(deprecatedPanicMsg("Cache",
			"NewAgent(name, prompt, model, WithCache(cache))"))
	}
}

// deprecatedPanicMsg 生成统一的 panic 消息
func deprecatedPanicMsg(field, migration string) string {
	return fmt.Sprintf(
		"ReActConfig.%s is deprecated and no longer accepted in v1.0.0;\n"+
			"use %s instead.\n"+
			"See: %s",
		field, migration, migrationGuideURL,
	)
}
