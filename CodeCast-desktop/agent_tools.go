package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// executeTool dispatches a tool call to the appropriate implementation
func (pool *AgentPool) executeTool(agent *SubAgent, tc ToolCall) ToolResult {
	event := AgentEvent{
		AgentID:  agent.ID,
		Type:     "tool_use",
		ToolName: tc.Name,
		Turn:     agent.TurnCount,
		MaxTurns: agent.MaxTurns,
	}
	wailsRuntime.EventsEmit(pool.app.ctx, "agent:event", event)

	var result ToolResult
	switch tc.Name {
	case "read_file":
		result = pool.toolReadFile(agent, tc.Args)
	case "write_file":
		result = pool.toolWriteFile(agent, tc.Args)
	case "edit_file":
		result = pool.toolEditFile(agent, tc.Args)
	case "run_command":
		result = pool.toolRunCommand(agent, tc.Args)
	case "search":
		result = pool.toolSearch(agent, tc.Args)
	case "web_fetch":
		result = pool.toolWebFetch(agent, tc.Args)
	default:
		result = ToolResult{
			Content: fmt.Sprintf("未知工具: %s", tc.Name),
			IsError: true,
		}
	}
	result.ToolCallID = tc.ID
	return result
}

// canWriteFile checks if the agent is allowed to write to the given path
func canWriteFile(agent *SubAgent, path string) bool {
	if len(agent.FilesScope) == 0 {
		return true
	}
	absPath := filepath.Clean(path)
	for _, scope := range agent.FilesScope {
		scopeAbs := filepath.Clean(scope)
		if absPath == scopeAbs || strings.HasPrefix(absPath, scopeAbs+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// resolvePath resolves a path relative to the project directory
func (pool *AgentPool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	pool.app.mu.Lock()
	var projectPath string
	if len(pool.app.projects) > 0 {
		projectPath = pool.app.projects[0].Path
	}
	pool.app.mu.Unlock()

	if projectPath != "" {
		return filepath.Join(projectPath, path)
	}
	return filepath.Clean(path)
}

// --- read_file ---

func (pool *AgentPool) toolReadFile(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	path := pool.resolvePath(args.Path)
	info, err := os.Stat(path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("读取文件失败: %v", err), IsError: true}
	}

	if info.Size() > int64(MaxReadFileSize) {
		return ToolResult{Content: fmt.Sprintf("文件过大 (%s)，上限 4MB", formatFileSize(info.Size())), IsError: true}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("读取文件失败: %v", err), IsError: true}
	}

	return ToolResult{Content: string(data), IsError: false}
}

// --- write_file ---

func (pool *AgentPool) toolWriteFile(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	path := pool.resolvePath(args.Path)

	if !canWriteFile(agent, path) {
		return ToolResult{
			Content: fmt.Sprintf("无权写入文件 %s，不在 files_scope 范围内", args.Path),
			IsError: true,
		}
	}

	if len(args.Content) > MaxWriteFileSize {
		return ToolResult{Content: "内容过大，超过 10MB 上限", IsError: true}
	}

	// Layer 2: Acquire file-level write lock
	pool.AcquireFileLock(path)
	defer pool.ReleaseFileLock(path)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ToolResult{Content: fmt.Sprintf("创建目录失败: %v", err), IsError: true}
	}

	if err := os.WriteFile(path, []byte(args.Content), 0644); err != nil {
		return ToolResult{Content: fmt.Sprintf("写入文件失败: %v", err), IsError: true}
	}

	return ToolResult{Content: fmt.Sprintf("已写入文件: %s (%d bytes)", args.Path, len(args.Content)), IsError: false}
}

// --- edit_file ---

func (pool *AgentPool) toolEditFile(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	path := pool.resolvePath(args.Path)

	if !canWriteFile(agent, path) {
		return ToolResult{
			Content: fmt.Sprintf("无权编辑文件 %s，不在 files_scope 范围内", args.Path),
			IsError: true,
		}
	}

	// Layer 2: Acquire file-level write lock
	pool.AcquireFileLock(path)
	defer pool.ReleaseFileLock(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("读取文件失败: %v", err), IsError: true}
	}

	content := string(data)
	if !strings.Contains(content, args.OldString) {
		return ToolResult{Content: "未找到要替换的文本", IsError: true}
	}

	count := strings.Count(content, args.OldString)
	if count > 1 {
		return ToolResult{Content: fmt.Sprintf("找到 %d 处匹配，请提供更精确的文本以唯一定位", count), IsError: true}
	}

	newContent := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return ToolResult{Content: fmt.Sprintf("写入文件失败: %v", err), IsError: true}
	}

	return ToolResult{Content: fmt.Sprintf("已编辑文件: %s", args.Path), IsError: false}
}

// --- run_command ---

func (pool *AgentPool) toolRunCommand(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Command    string `json:"command"`
		WorkingDir string `json:"working_dir"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	if err := validateCommand(agent, args.Command); err != nil {
		return ToolResult{Content: fmt.Sprintf("命令被拒绝: %v", err), IsError: true}
	}

	args.Command = sanitizeCommandForExecution(args.Command)

	workDir := args.WorkingDir
	if workDir == "" {
		pool.app.mu.Lock()
		if len(pool.app.projects) > 0 {
			workDir = pool.app.projects[0].Path
		}
		pool.app.mu.Unlock()
	}

	var shell, flag string
	if runtime.GOOS == "windows" {
		shell = "cmd"
		flag = "/C"
	} else {
		shell = os.Getenv("SHELL")
		if shell == "" {
			if runtime.GOOS == "darwin" {
				shell = "/bin/zsh"
			} else {
				shell = "/bin/bash"
			}
		}
		flag = "-c"
	}

	ctx, cancel := context.WithTimeout(agent.ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, flag, args.Command)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = append(os.Environ(), pool.app.getCustomEnvVars()...)

	output, err := cmd.CombinedOutput()
	result := string(output)

	if len(result) > 50000 {
		result = result[:50000] + "\n...[输出截断]"
	}

	if ctx.Err() == context.DeadlineExceeded {
		return ToolResult{Content: result + "\n[命令执行超时: 5分钟]", IsError: true}
	}
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("%s\n[错误: %v]", result, err), IsError: true}
	}

	return ToolResult{Content: result, IsError: false}
}

// --- search ---

func (pool *AgentPool) toolSearch(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	searchPath := args.Path
	if searchPath == "" {
		pool.app.mu.Lock()
		if len(pool.app.projects) > 0 {
			searchPath = pool.app.projects[0].Path
		}
		pool.app.mu.Unlock()
	}
	if searchPath == "" {
		return ToolResult{Content: "未指定搜索路径且无项目目录", IsError: true}
	}

	// Build grep/findstr command based on OS
	var cmdStr string
	if runtime.GOOS == "windows" {
		// Use findstr on Windows
		includeGlob := "*"
		if args.Include != "" {
			includeGlob = args.Include
		}
		cmdStr = fmt.Sprintf("findstr /s /n /i \"%s\" \"%s\\%s\"", args.Pattern, searchPath, includeGlob)
	} else {
		// Use grep on Unix
		includeFlag := ""
		if args.Include != "" {
			includeFlag = fmt.Sprintf("--include='%s' ", args.Include)
		}
		cmdStr = fmt.Sprintf("grep -rn %s\"%s\" \"%s\" 2>/dev/null | head -100", includeFlag, args.Pattern, searchPath)
	}

	var shell, flag string
	if runtime.GOOS == "windows" {
		shell = "cmd"
		flag = "/C"
	} else {
		shell = "/bin/bash"
		flag = "-c"
	}

	ctx, cancel := context.WithTimeout(agent.ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, flag, cmdStr)
	output, err := cmd.CombinedOutput()
	result := string(output)

	if len(result) > 50000 {
		result = result[:50000] + "\n...[结果截断]"
	}

	if err != nil && result == "" {
		return ToolResult{Content: "未找到匹配内容", IsError: false}
	}

	return ToolResult{Content: result, IsError: false}
}

// --- web_fetch ---

func (pool *AgentPool) toolWebFetch(agent *SubAgent, argsJSON string) ToolResult {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("参数解析失败: %v", err), IsError: true}
	}

	ctx, cancel := context.WithTimeout(agent.ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", args.URL, nil)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("创建请求失败: %v", err), IsError: true}
	}
	req.Header.Set("User-Agent", "CodeCast-Agent/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("请求失败: %v", err), IsError: true}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("读取响应失败: %v", err), IsError: true}
	}

	if resp.StatusCode != http.StatusOK {
		preview := string(body[:min(len(body), 500)])
		return ToolResult{Content: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, preview), IsError: true}
	}

	content := string(body)
	if len(content) > 50000 {
		content = content[:50000] + "\n...[内容截断]"
	}

	return ToolResult{Content: content, IsError: false}
}
