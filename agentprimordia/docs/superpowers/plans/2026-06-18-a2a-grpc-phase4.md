# A2A gRPC/protobuf Phase 4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 同步更新 api-reference.md 文档，补充 benchmark 对比 HTTP JSON-RPC 与 gRPC，完成最终完整性验证。

**Architecture:** 文档使用 `pkg/ap` 公共 API 示例；benchmark 使用 `bufconn` 测试 gRPC、`httptest` 测试 HTTP，对比 `task/create` 吞吐。

**Tech Stack:** Go 1.26, testing, gRPC, net/http/httptest

---

## 文件结构

| 文件 | 类型 | 说明 |
|---|---|---|
| `ecosystem/docs/api-reference.md` | 修改 | 更新 A2A 协议章节，补充 gRPC API 与公共 API |
| `internal/agent/a2a/bench_test.go` | 创建 | HTTP 与 gRPC 性能基准测试 |

---

## Task 1: 更新 api-reference.md

**Files:**
- Modify: `ecosystem/docs/api-reference.md`

- [x] **Step 1: 目录增加 gRPC API 链接**
- [x] **Step 2: A2A Server 创建示例改为 `pkg/ap`**
- [x] **Step 3: 新增 A2A gRPC API 章节**
- [x] **Step 4: 更新 task/cancel 响应说明**
- [x] **Step 5: 更新 SSE 不存在任务行为说明**

---

## Task 2: 添加 benchmark

**Files:**
- Create: `internal/agent/a2a/bench_test.go`

- [ ] **Step 1: 创建 A2AService / gRPC / HTTP 基准测试**

```go
package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	a2av1 "agentprimordia/internal/agent/a2a/proto/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func BenchmarkA2AService_CreateTask(b *testing.B) {
	card := NewAgentCard("agent", "Agent")
	service := NewA2AService(card, NewTaskManager())
	msg := &A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.CreateTask(ctx, &CreateTaskRequest{Message: msg})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGRPC_CreateTask(b *testing.B) {
	card := NewAgentCard("agent", "Agent")
	service := NewA2AService(card, NewTaskManager())
	server := NewGRPCServer(service)
	lis := bufconn.Listen(1024 * 1024)
	go server.Serve(lis)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	client := a2av1.NewA2AServiceClient(conn)
	msg := toProtoMessage(&A2AMessage{Role: "user", Parts: []Part{NewTextPart("hello")}})
	reqCtx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.CreateTask(reqCtx, &a2av1.CreateTaskRequest{Message: msg})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHTTP_CreateTask(b *testing.B) {
	card := NewAgentCard("agent", "Agent")
	server := NewA2AServer(NewTaskManager(), WithCard(card))
	handler := server.Handler()

	params, _ := json.Marshal(map[string]any{
		"message": map[string]string{"role": "user"},
	})
	body, _ := json.Marshal(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "task/create", Params: params})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}
```

注意：需要 import `net`、`net/http`。

- [ ] **Step 2: 运行 benchmark**

Run: `go test ./internal/agent/a2a/ -bench=. -benchtime=1s -run=^$`
Expected: 三个 benchmark 均成功运行并输出 ns/op。

---

## Task 3: 最终完整性验证

**Files:** 全部 Phase 1/2/3/4 改动

- [ ] **Step 1: 全包测试**

Run: `go test ./internal/agent/a2a/... -timeout 60s`
Expected: 通过。

- [ ] **Step 2: 全项目构建**

Run: `go build ./...`
Expected: 成功。

- [ ] **Step 3: 检查无占位符**

Run: `grep -R "TODO\|TBD\|FIXME" internal/agent/a2a/grpc_*.go internal/agent/a2a/server.go internal/agent/a2a/service.go pkg/a2a.go bench_test.go || true`
Expected: 无匹配。
