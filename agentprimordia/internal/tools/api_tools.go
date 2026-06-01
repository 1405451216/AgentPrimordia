package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 30 * time.Second
	defaultGitLogCount = "-20"
)

// ===== HTTP 增强客户端 =====

// HTTPClientTool 是增强的 HTTP 客户端工具
type HTTPClientTool struct {
	name        string
	description string
	client      *http.Client
}

// NewHTTPClientTool 创建新的 HTTP 客户端工具
func NewHTTPClientTool() *HTTPClientTool {
	return &HTTPClientTool{
		name: "http_client",
		description: `增强型 HTTP 客户端，支持 REST API 调用、认证、重试和响应处理。
功能：
- GET/POST/PUT/DELETE/PATCH 请求
- 自动 JSON 请求/响应处理
- Bearer Token 和 Basic Auth 认证
- 请求重试和超时控制
- 响应状态码检查和错误处理

参数：
- method (required): HTTP 方法 [GET|POST|PUT|DELETE|PATCH]
- url (required): 请求 URL
- headers (optional): 请求头（JSON 对象）
- body (optional): 请求体（字符串或 JSON 对象）
- auth_type (optional): 认证类型 [bearer|basic|none]
- auth_token (optional): 认证令牌
- timeout (optional): 超时时间（秒）
- max_retries (optional): 最大重试次数`,
		client: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

func (t *HTTPClientTool) Name() string        { return t.name }
func (t *HTTPClientTool) Description() string { return t.description }

func (t *HTTPClientTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"method": {"type": "string", "enum": ["GET", "POST", "PUT", "DELETE", "PATCH"]},
			"url": {"type": "string"},
			"headers": {"type": "object"},
			"body": {"type": ["string", "object"]},
			"auth_type": {"type": "string", "enum": ["bearer", "basic", "none"]},
			"auth_token": {"type": "string"},
			"timeout": {"type": "number"},
			"max_retries": {"type": "number"}
		},
		"required": ["method", "url"]
	}`)
}

func (t *HTTPClientTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var params map[string]any
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse parameters error: %w", err)
	}

	method, ok := params["method"].(string)
	if !ok {
		return NewErrorResult("parameter 'method' must be a string"), nil
	}
	requestURL, ok := params["url"].(string)
	if !ok {
		return NewErrorResult("parameter 'url' must be a string"), nil
	}

	timeout := defaultHTTPTimeout
	if to, ok := params["timeout"].(float64); ok {
		timeout = time.Duration(to) * time.Second
	}

	client := &http.Client{Timeout: timeout}

	var bodyReader io.Reader
	if body, ok := params["body"]; ok && body != nil {
		var bodyBytes []byte
		switch v := body.(type) {
		case string:
			bodyBytes = []byte(v)
		default:
			var err error
			bodyBytes, err = json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("marshal body error: %w", err)
			}
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if headers, ok := params["headers"].(map[string]any); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	authType := "none"
	if at, ok := params["auth_type"].(string); ok {
		authType = at
	}
	authToken := ""
	if at, ok := params["auth_token"].(string); ok {
		authToken = at
	}

	switch authType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+authToken)
	case "basic":
		req.Header.Set("Authorization", "Basic "+authToken)
	}

	maxRetries := 0
	if mr, ok := params["max_retries"].(float64); ok {
		maxRetries = int(mr)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			return nil, fmt.Errorf("request failed after %d attempts: %w", attempt+1, err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		result := map[string]any{
			"status_code": resp.StatusCode,
			"status":      resp.Status,
			"headers":     extractResponseHeaders(resp.Header),
			"body":        string(respBody),
		}

		output, _ := json.MarshalIndent(result, "", "  ")
		return &Result{
			Content: string(output),
			Metadata: map[string]any{
				"attempt": strconv.Itoa(attempt + 1),
				"url":     requestURL,
				"method":  method,
			},
		}, nil
	}

	return nil, fmt.Errorf("all retries exhausted: %w", lastErr)
}

// ===== Git 操作工具 =====

// GitTool 是 Git 版本控制操作工具
type GitTool struct {
	name        string
	description string
	workDir     string
}

// NewGitTool 创建新的 Git 工具
func NewGitTool(workDir string) *GitTool {
	return &GitTool{
		name: "git_tool",
		description: `Git 版本控制操作工具，支持常用 Git 命令。
功能：克隆仓库、查看状态、提交更改、创建分支、合并分支等`,
		workDir: workDir,
	}
}

func (t *GitTool) Name() string        { return t.name }
func (t *GitTool) Description() string { return t.description }

func (t *GitTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["clone", "status", "log", "diff", "commit", "branch", "checkout", "pull", "push", "add"]},
			"repo_url": {"type": "string"},
			"branch": {"type": "string"},
			"message": {"type": "string"},
			"files": {"type": "array"},
			"options": {"type": "array"}
		},
		"required": ["action"]
	}`)
}

func (t *GitTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var params map[string]any
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse parameters error: %w", err)
	}

	action, ok := params["action"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'action' must be a string")
	}

	switch action {
	case "clone":
		return t.cloneRepo(ctx, params)
	case "status":
		return t.gitStatus(ctx)
	case "log":
		return t.gitLog(ctx, params)
	case "diff":
		return t.gitDiff(ctx, params)
	case "commit":
		return t.gitCommit(ctx, params)
	case "branch":
		return t.branchListCreate(ctx, params)
	case "checkout":
		return t.checkoutBranch(ctx, params)
	case "pull":
		return t.pullChanges(ctx)
	case "push":
		return t.pushChanges(ctx)
	case "add":
		return t.addFiles(ctx, params)
	default:
		return nil, fmt.Errorf("unknown git action: %s", action)
	}
}

func (t *GitTool) cloneRepo(ctx context.Context, params map[string]any) (*Result, error) {
	repoURL, ok := params["repo_url"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'repo_url' must be a string")
	}
	if err := validateGitURL(repoURL); err != nil {
		return nil, err
	}

	targetDir := t.workDir
	if td, ok := params["target_dir"].(string); ok {
		targetDir = td
	}

	args := []string{"clone", "--", repoURL, targetDir}
	if branch, ok := params["branch"].(string); ok {
		args = []string{"clone", "-b", branch, "--", repoURL, targetDir}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git clone error: %w\n%s", err, string(output))
	}

	return &Result{
		Content: fmt.Sprintf("Repository cloned successfully to: %s\n\n%s", targetDir, string(output)),
		Metadata: map[string]any{
			"action":     "clone",
			"repo_url":   repoURL,
			"target_dir": targetDir,
		},
	}, nil
}

func (t *GitTool) gitStatus(ctx context.Context) (*Result, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = t.workDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status error: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	statusMap := make(map[string][]string)
	statusMap["modified"] = []string{}
	statusMap["added"] = []string{}
	statusMap["deleted"] = []string{}
	statusMap["untracked"] = []string{}

	for _, line := range lines {
		if line == "" {
			continue
		}
		status := strings.TrimSpace(line[:2])
		file := strings.TrimSpace(line[3:])
		switch {
		case strings.Contains(status, "M"):
			statusMap["modified"] = append(statusMap["modified"], file)
		case strings.Contains(status, "A"):
			statusMap["added"] = append(statusMap["added"], file)
		case strings.Contains(status, "D"):
			statusMap["deleted"] = append(statusMap["deleted"], file)
		case status == "??":
			statusMap["untracked"] = append(statusMap["untracked"], file)
		}
	}

	resultJSON, _ := json.MarshalIndent(statusMap, "", "  ")
	return &Result{
		Content: string(resultJSON),
		Metadata: map[string]any{
			"total_changes": strconv.Itoa(len(lines)),
		},
	}, nil
}

func (t *GitTool) gitLog(ctx context.Context, params map[string]any) (*Result, error) {
	args := []string{"log", "--oneline", defaultGitLogCount}
	if n, ok := params["count"].(float64); ok {
		args[2] = fmt.Sprintf("-%d", int(n))
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = t.workDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log error: %w", err)
	}

	commits := []map[string]string{}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			commits = append(commits, map[string]string{"hash": parts[0], "message": parts[1]})
		}
	}

	resultJSON, _ := json.MarshalIndent(commits, "", "  ")
	return &Result{Content: string(resultJSON)}, nil
}

func (t *GitTool) gitDiff(ctx context.Context, params map[string]any) (*Result, error) {
	args := []string{"diff", "--"}
	if file, ok := params["file"].(string); ok {
		args = append(args, file)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = t.workDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff error: %w", err)
	}

	return &Result{Content: string(output)}, nil
}

func (t *GitTool) gitCommit(ctx context.Context, params map[string]any) (*Result, error) {
	message, ok := params["message"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'message' must be a string")
	}

	addCmd := exec.CommandContext(ctx, "git", "add", ".")
	addCmd.Dir = t.workDir
	if _, err := addCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git add error: %w", err)
	}

	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commitCmd.Dir = t.workDir
	output, err := commitCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git commit error: %w\n%s", err, string(output))
	}

	return &Result{
		Content:  fmt.Sprintf("Commit created successfully:\n%s", string(output)),
		Metadata: map[string]any{"message": message},
	}, nil
}

func (t *GitTool) branchListCreate(ctx context.Context, params map[string]any) (*Result, error) {
	cmd := exec.CommandContext(ctx, "git", "branch", "-a")
	cmd.Dir = t.workDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git branch error: %w", err)
	}

	branches := []string{}
	for _, b := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		branches = append(branches, strings.TrimSpace(b))
	}

	resultJSON, _ := json.MarshalIndent(map[string]any{"branches": branches}, "", "  ")
	return &Result{Content: string(resultJSON)}, nil
}

func (t *GitTool) checkoutBranch(ctx context.Context, params map[string]any) (*Result, error) {
	branch, ok := params["branch"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'branch' must be a string")
	}
	cmd := exec.CommandContext(ctx, "git", "checkout", "--", branch)
	cmd.Dir = t.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git checkout error: %w\n%s", err, string(output))
	}
	return &Result{Content: fmt.Sprintf("Switched to branch '%s':\n%s", branch, string(output))}, nil
}

func (t *GitTool) pullChanges(ctx context.Context) (*Result, error) {
	cmd := exec.CommandContext(ctx, "git", "pull")
	cmd.Dir = t.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git pull error: %w\n%s", err, string(output))
	}
	return &Result{Content: string(output)}, nil
}

func (t *GitTool) pushChanges(ctx context.Context) (*Result, error) {
	cmd := exec.CommandContext(ctx, "git", "push")
	cmd.Dir = t.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git push error: %w\n%s", err, string(output))
	}
	return &Result{Content: string(output)}, nil
}

func (t *GitTool) addFiles(ctx context.Context, params map[string]any) (*Result, error) {
	files, _ := params["files"].([]any)
	args := append([]string{"add", "--"}, toStringSlice(files)...)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = t.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git add error: %w\n%s", err, string(output))
	}
	return &Result{Content: string(output)}, nil
}

// ===== 搜索工具 =====

// SearchTool 是网络搜索和信息检索工具
type SearchTool struct {
	name        string
	description string
}

// NewSearchTool 创建新的搜索工具
func NewSearchTool() *SearchTool {
	return &SearchTool{
		name: "web_search",
		description: `网络搜索和信息检索工具。
功能：搜索网页内容、获取页面信息、提取关键信息`,
	}
}

func (t *SearchTool) Name() string        { return t.name }
func (t *SearchTool) Description() string { return t.description }

func (t *SearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string"},
			"engine": {"type": "string", "enum": ["general", "code", "academic", "news"]},
			"num_results": {"type": "number"},
			"language": {"type": "string"}
		},
		"required": ["query"]
	}`)
}

func (t *SearchTool) Execute(ctx context.Context, input json.RawMessage) (*Result, error) {
	var params map[string]any
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("parse parameters error: %w", err)
	}

	_, ok := params["query"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'query' must be a string")
	}

	return NewErrorResult("search tool is not connected to a real search engine; configure a search provider before use"), nil
}

// SearchResultItem 是搜索结果项
type SearchResultItem struct {
	Title          string  `json:"title"`
	URL            string  `json:"url"`
	Snippet        string  `json:"snippet"`
	RelevanceScore float64 `json:"relevance_score"`
}

// ===== 辅助函数 =====

// validateGitURL 验证 Git URL 格式，防止参数注入
func validateGitURL(url string) error {
	allowed := []string{"http://", "https://", "git://", "ssh://", "git@"}
	for _, prefix := range allowed {
		if strings.HasPrefix(url, prefix) {
			return nil
		}
	}
	return fmt.Errorf("invalid git URL: must start with http://, https://, git://, ssh://, or git@")
}

func extractResponseHeaders(header http.Header) map[string]string {
	result := make(map[string]string)
	for key, values := range header {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

func toStringSlice(items []any) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = fmt.Sprintf("%v", item)
	}
	return result
}
