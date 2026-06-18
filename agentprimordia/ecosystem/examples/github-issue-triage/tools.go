// Phase 18: GitHub Issue Triage Demo - Tools
//
// 实现 ap.Tool 接口的 3 个 GitHub 工具：
//   - listIssuesTool:  列出 issues
//   - readIssueTool:   读取 issue 详情
//   - addLabelTool:    给 issue 添加 label
//
// 所有工具通过 HTTP 调用 mock server（生产环境可换成真实 GitHub API）。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ap "agentprimordia/pkg"
)

// apiBase 是 mock GitHub server 的基础 URL，由 main 在启动时注入
var apiBase string

// httpClient 共享给所有工具，避免每次都新建 client
var httpClient = &http.Client{Timeout: 10 * time.Second}

// ===== listIssuesTool =====

type listIssuesTool struct{}

func (listIssuesTool) Name() string { return "list_issues" }

func (listIssuesTool) Description() string {
	return `列出 GitHub repo 的 issues。

输入参数:
  state: "open" | "closed" | "all"（可选，默认 "open"）

返回: JSON 数组，每项包含 number, title, state, labels, author, created_at
   示例: [{"number": 1, "title": "panic in main loop", "state": "open", "labels": ["bug"]}]`
}

func (listIssuesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"state": {
				"type": "string",
				"enum": ["open", "closed", "all"],
				"description": "Issue 状态过滤",
				"default": "open"
			}
		}
	}`)
}

func (listIssuesTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params struct {
		State string `json:"state"`
	}
	if len(args) > 0 {
		_ = json.Unmarshal(args, &params)
	}
	if params.State == "" {
		params.State = "open"
	}

	url := fmt.Sprintf("%s/repos/owner/repo/issues?state=%s", apiBase, params.State)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("list_issues http error: %v", err)), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ap.NewToolErrorResult(fmt.Sprintf("list_issues status=%d body=%s", resp.StatusCode, string(body))), nil
	}
	return ap.NewToolResult(string(body)), nil
}

// ===== readIssueTool =====

type readIssueTool struct{}

func (readIssueTool) Name() string { return "read_issue" }

func (readIssueTool) Description() string {
	return `读取单个 GitHub issue 的完整详情。

输入参数:
  issue_number: 整数，必填

返回: JSON 对象，包含 number, title, body, labels, author, created_at
   示例: {"number": 1, "title": "...", "body": "...", "labels": [], "author": "alice"}`
}

func (readIssueTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"issue_number": {
				"type": "integer",
				"description": "Issue 编号"
			}
		},
		"required": ["issue_number"]
	}`)
}

func (readIssueTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params struct {
		IssueNumber int `json:"issue_number"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("read_issue invalid args: %v", err)), nil
	}
	if params.IssueNumber == 0 {
		return ap.NewToolErrorResult("read_issue requires issue_number"), nil
	}

	url := fmt.Sprintf("%s/repos/owner/repo/issues/%d", apiBase, params.IssueNumber)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("read_issue http error: %v", err)), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ap.NewToolErrorResult(fmt.Sprintf("read_issue status=%d body=%s", resp.StatusCode, string(body))), nil
	}
	return ap.NewToolResult(string(body)), nil
}

// ===== addLabelTool =====

type addLabelTool struct{}

func (addLabelTool) Name() string { return "add_label" }

func (addLabelTool) Description() string {
	return `给 GitHub issue 添加一个或多个 label。

输入参数:
  issue_number: 整数，必填
  labels: 字符串数组，必填，例如 ["bug", "priority:high"]

返回: JSON 对象 {ok, issue_number, labels_applied, current_labels}`
}

func (addLabelTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"issue_number": {
				"type": "integer",
				"description": "Issue 编号"
			},
			"labels": {
				"type": "array",
				"items": {"type": "string"},
				"description": "要添加的 label 列表"
			}
		},
		"required": ["issue_number", "labels"]
	}`)
}

func (addLabelTool) Execute(ctx context.Context, args json.RawMessage) (*ap.ToolResult, error) {
	var params struct {
		IssueNumber int      `json:"issue_number"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("add_label invalid args: %v", err)), nil
	}
	if params.IssueNumber == 0 {
		return ap.NewToolErrorResult("add_label requires issue_number"), nil
	}
	if len(params.Labels) == 0 {
		return ap.NewToolErrorResult("add_label requires at least one label"), nil
	}

	body, _ := json.Marshal(map[string]any{"labels": params.Labels})
	url := fmt.Sprintf("%s/repos/owner/repo/issues/%d/labels", apiBase, params.IssueNumber)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return ap.NewToolErrorResult(fmt.Sprintf("add_label http error: %v", err)), nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ap.NewToolErrorResult(fmt.Sprintf("add_label status=%d body=%s", resp.StatusCode, string(respBody))), nil
	}
	return ap.NewToolResult(string(respBody)), nil
}

// registryFromTools 用最简方式注册工具到 ap.ToolRegistry
// 不使用 DefaultToolkit 因为本 demo 只需要自定义工具，不需要 FS/Shell/Web
func registryFromTools(tools ...ap.Tool) (*ap.ToolRegistry, error) {
	registry := ap.NewToolRegistry()
	for _, t := range tools {
		if err := registry.Register(t); err != nil {
			return nil, fmt.Errorf("failed to register tool %s: %w", t.Name(), err)
		}
	}
	return registry, nil
}

// formatIssueBrief 把 Issue 简化成单行字符串（用于最终报告输出）
func formatIssueBrief(iss Issue) string {
	return fmt.Sprintf("#%-3d %-12s | labels=%-30s | %s",
		iss.Number,
		firstLabel(iss.Labels),
		strings.Join(iss.Labels, ","),
		truncate(iss.Title, 50),
	)
}

func firstLabel(labels []string) string {
	if len(labels) == 0 {
		return "(none)"
	}
	return labels[0]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
