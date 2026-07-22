package git

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	ap "agentprimordia/pkg"
)

// Plugin 是 Git 版本控制操作插件
type Plugin struct {
	tool *GitTool
}

// New 创建新的 Git 插件实例
func New() *Plugin {
	return &Plugin{tool: &GitTool{}}
}

// Name 返回插件名称
func (p *Plugin) Name() string { return "git" }

// Version 返回插件版本
func (p *Plugin) Version() string { return "0.7.0" }

// Tools 返回插件提供的工具列表
func (p *Plugin) Tools() []ap.Tool {
	return []ap.Tool{p.tool}
}

// Init 初始化插件（Git 工具无需额外配置）
func (p *Plugin) Init(config map[string]any) error { return nil }

// Close 关闭插件资源
func (p *Plugin) Close() error { return nil }

// GitTool 是 Git 版本控制操作工具
type GitTool struct{}

// Name 返回工具名称
func (t *GitTool) Name() string { return "git_tool" }

// Description 返回工具描述
func (t *GitTool) Description() string {
	return `Git 版本控制操作工具，支持 clone/status/log/diff/commit/branch 等操作。

功能：
- status: 查看工作区状态
- log: 查看提交历史
- diff: 查看未暂存的变更
- branch: 列出/创建/删除分支
- commit: 提交暂存区的变更
- add: 暂存文件

参数：
- action (required): 操作类型 [status|log|diff|branch|commit|add]
- args: Git 命令参数（如 ["--oneline", "-5"] 用于 log）
- message: 提交消息（commit 操作必需）
- workdir: Git 仓库目录（默认当前目录）`
}

// Parameters 返回工具参数的 JSON Schema
func (t *GitTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["status", "log", "diff", "branch", "commit", "add"]},
			"args": {"type": "array", "items": {"type": "string"}, "description": "Git 命令参数"},
			"message": {"type": "string", "description": "提交消息（commit 操作必需）"},
			"workdir": {"type": "string", "description": "Git 仓库目录（默认当前目录）"}
		},
		"required": ["action"]
	}`)
}

// Category 返回工具分类
func (t *GitTool) Category() string { return "vcs" }

// Execute 执行 Git 操作
func (t *GitTool) Execute(ctx context.Context, input json.RawMessage) (*ap.ToolResult, error) {
	var params map[string]any
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("解析参数错误: %w", err)
	}

	action, _ := params["action"].(string)
	if action == "" {
		return ap.NewToolErrorResult("参数 'action' 不能为空"), nil
	}

	workdir, _ := params["workdir"].(string)
	argsAny, _ := params["args"].([]any)
	args := make([]string, 0, len(argsAny))
	for _, a := range argsAny {
		if s, ok := a.(string); ok {
			args = append(args, s)
		}
	}

	var gitArgs []string
	switch action {
	case "status":
		gitArgs = append([]string{"status", "--short"}, args...)
	case "log":
		gitArgs = append([]string{"log"}, args...)
	case "diff":
		gitArgs = append([]string{"diff"}, args...)
	case "branch":
		if len(args) == 0 {
			gitArgs = []string{"branch", "--list"}
		} else {
			gitArgs = append([]string{"branch"}, args...)
		}
	case "commit":
		message, _ := params["message"].(string)
		if message == "" {
			return ap.NewToolErrorResult("commit 操作需要 'message' 参数"), nil
		}
		gitArgs = append([]string{"commit", "-m", message}, args...)
	case "add":
		if len(args) == 0 {
			return ap.NewToolErrorResult("add 操作需要 'args' 参数指定要暂存的文件"), nil
		}
		gitArgs = append([]string{"add"}, args...)
	default:
		return ap.NewToolErrorResult(fmt.Sprintf("未知操作: %s，支持 status/log/diff/branch/commit/add", action)), nil
	}

	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	output, err := cmd.CombinedOutput()

	result := map[string]any{
		"action":   action,
		"output":   strings.TrimSpace(string(output)),
		"exit_code": cmd.ProcessState.ExitCode(),
	}
	if err != nil && cmd.ProcessState.ExitCode() != 0 {
		result["error"] = err.Error()
	}

	outputJSON, _ := json.MarshalIndent(result, "", "  ")
	return &ap.ToolResult{Content: string(outputJSON)}, nil
}
