//go:build ignore

// mock_server.go 提供测试用的模拟 MCP 服务器。
// 该文件使用 go:build ignore 标签，不会被正常编译到包中。
// 测试时通过 TestMain 编译为独立可执行文件运行。
//
// 该服务器通过 stdin/stdout 进行 JSON-RPC 2.0 通信，
// 模拟真实 MCP 服务器的行为。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ===== 自包含的 JSON-RPC 类型 =====

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// 增大缓冲区以支持较大的请求
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		resp := handleRequest(&req)
		if resp != nil {
			respBody, _ := json.Marshal(resp)
			fmt.Println(string(respBody))
		}
	}
}

// handleRequest 处理模拟请求
func handleRequest(req *request) *response {
	// 通知没有 ID，不需要响应
	if req.ID == 0 {
		return nil
	}

	switch req.Method {
	case "initialize":
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: json.RawMessage(`{
				"protocolVersion": "2024-11-05",
				"capabilities": {
					"tools": {"listChanged": true},
					"resources": {"subscribe": true, "listChanged": true},
					"prompts": {"listChanged": true}
				},
				"serverInfo": {
					"name": "mock-mcp-server",
					"version": "1.0.0"
				}
			}`),
		}

	case "ping":
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{}`),
		}

	case "tools/list":
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: json.RawMessage(`{
				"tools": [
					{
						"name": "echo",
						"description": "回显输入文本",
						"inputSchema": {
							"type": "object",
							"properties": {
								"message": {
									"type": "string",
									"description": "要回显的文本"
								}
							},
							"required": ["message"]
						}
					},
					{
						"name": "add",
						"description": "计算两个数的和",
						"inputSchema": {
							"type": "object",
							"properties": {
								"a": {"type": "number"},
								"b": {"type": "number"}
							},
							"required": ["a", "b"]
						}
					}
				]
			}`),
		}

	case "tools/call":
		return handleToolCall(req)

	case "resources/list":
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: json.RawMessage(`{
				"resources": [
					{
						"uri": "file:///data/config.json",
						"name": "配置文件",
						"description": "应用配置",
						"mimeType": "application/json"
					}
				]
			}`),
		}

	case "resources/read":
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: json.RawMessage(`{
				"contents": [
					{
						"uri": "file:///data/config.json",
						"mimeType": "application/json",
						"text": "{\"key\": \"value\"}"
					}
				]
			}`),
		}

	case "prompts/list":
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: json.RawMessage(`{
				"prompts": [
					{
						"name": "greet",
						"description": "问候提示词",
						"arguments": [
							{"name": "name", "description": "姓名", "required": true}
						]
					}
				]
			}`),
		}

	case "prompts/get":
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: json.RawMessage(`{
				"messages": [
					{
						"role": "user",
						"content": {
							"type": "text",
							"text": "你好，欢迎使用 MCP！"
						}
					}
				]
			}`),
		}

	case "shutdown":
		os.Exit(0)
		return nil

	default:
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32601,
				Message: fmt.Sprintf("方法 %q 未找到", req.Method),
			},
		}
	}
}

// handleToolCall 处理tool调用请求
func handleToolCall(req *request) *response {
	// 解析参数
	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "无效参数"},
		}
	}

	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32602, Message: "参数解析失败"},
		}
	}

	switch params.Name {
	case "echo":
		msg, _ := params.Arguments["message"].(string)
		result, _ := json.Marshal(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": msg},
			},
		})
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(result),
		}

	case "add":
		a, _ := params.Arguments["a"].(float64)
		b, _ := params.Arguments["b"].(float64)
		result, _ := json.Marshal(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("%.0f", a+b)},
			},
		})
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(result),
		}

	default:
		result, _ := json.Marshal(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("tool %q 不存在", params.Name)},
			},
			"isError": true,
		})
		return &response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(result),
		}
	}
}
