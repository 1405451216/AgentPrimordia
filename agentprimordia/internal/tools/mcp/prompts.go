package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// PromptHandler renders a prompt template with the given arguments into
// plain text that can be consumed by an LLM.
type PromptHandler func(ctx context.Context, args map[string]string) (string, error)

// promptEntry is an internal registered-prompt record.
type promptEntry struct {
	name    string
	handler PromptHandler
}

// promptRegistry manages Agent-exposed MCP prompts.
type promptRegistry struct {
	mu      sync.RWMutex
	prompts map[string]*promptEntry
}

// newPromptRegistry creates an empty prompt registry.
func newPromptRegistry() *promptRegistry {
	return &promptRegistry{
		prompts: make(map[string]*promptEntry),
	}
}

// Register adds or replaces a prompt entry keyed by its name.
func (r *promptRegistry) Register(name string, handler PromptHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prompts[name] = &promptEntry{name: name, handler: handler}
}

// Unregister removes a prompt by name.
func (r *promptRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.prompts, name)
}

// List returns all registered prompts in MCP wire format.
func (r *promptRegistry) List() []PromptDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]PromptDefinition, 0, len(r.prompts))
	for _, entry := range r.prompts {
		def := PromptDefinition{
			Name:        entry.name,
			Description: builtinPromptDescriptions[entry.name],
		}
		for argName, required := range builtinPromptArgSpecs[entry.name] {
			def.Arguments = append(def.Arguments, PromptArgument{
				Name:     argName,
				Required: required,
			})
		}
		result = append(result, def)
	}
	return result
}

// Get invokes the prompt handler for the given name and returns the fully rendered text.
func (r *promptRegistry) Get(ctx context.Context, name string, args map[string]string) (string, error) {
	r.mu.RLock()
	entry, ok := r.prompts[name]
	r.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("prompt not found: %s", name)
	}
	return entry.handler(ctx, args)
}

// Count returns the current number of registered prompts.
func (r *promptRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.prompts)
}

// Built-in prompt descriptions
var builtinPromptDescriptions = map[string]string{
	"summarize": "Summarize the core content of a topic",
	"analyze":   "Analyze given data and return structured insights",
	"plan":      "Make an actionable plan from a goal",
}

// built-in prompt argument specs
var builtinPromptArgSpecs = map[string]map[string]bool{
	"summarize": {"topic": true, "language": false},
	"analyze":   {"data": true, "format": false},
	"plan":      {"goal": true, "steps": false, "deadline": false},
}

// BuiltinPrompts provides default prompt templates for an Agent MCP server.
type BuiltinPrompts struct{}

// NewBuiltinPrompts returns the built-in prompt templates.
func NewBuiltinPrompts() *BuiltinPrompts {
	return &BuiltinPrompts{}
}

func summarizeHandler(ctx context.Context, args map[string]string) (string, error) {
	topic, ok := args["topic"]
	if !ok || strings.TrimSpace(topic) == "" {
		return "", fmt.Errorf("missing required arg: topic")
	}
	language := args["language"]
	if language == "" {
		language = "zh"
	}
	return fmt.Sprintf("Please summarize the core content of this topic in %s:\n\nTopic: %s", language, topic), nil
}

func analyzeHandler(ctx context.Context, args map[string]string) (string, error) {
	data, ok := args["data"]
	if !ok || strings.TrimSpace(data) == "" {
		return "", fmt.Errorf("missing required arg: data")
	}
	format := args["format"]
	if format == "" {
		format = "json"
	}
	return fmt.Sprintf("Please analyze the following data and return structured insights as %s:\n\n%s", format, data), nil
}

func planHandler(ctx context.Context, args map[string]string) (string, error) {
	goal, ok := args["goal"]
	if !ok || strings.TrimSpace(goal) == "" {
		return "", fmt.Errorf("missing required arg: goal")
	}
	steps := args["steps"]
	deadline := args["deadline"]
	var extra strings.Builder
	if steps != "" {
		extra.WriteString(fmt.Sprintf("\nSteps: %s", steps))
	}
	if deadline != "" {
		extra.WriteString(fmt.Sprintf("\nDeadline: %s", deadline))
	}
	return fmt.Sprintf("Please create an actionable plan for the following goal:\n\nGoal: %s%s", goal, extra.String()), nil
}

// RegisterTo installs all three built-in prompts into the given registry.
func (b *BuiltinPrompts) RegisterTo(r *promptRegistry) {
	r.Register("summarize", summarizeHandler)
	r.Register("analyze", analyzeHandler)
	r.Register("plan", planHandler)
}
