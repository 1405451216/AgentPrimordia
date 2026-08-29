# Admin API

Admin API 提供 HTTP 管理接口，用于查看 Agent、任务、工具、工作流和系统状态。

## 启动 Admin Server

```go
pool := NewPool(PoolConfig{...})
registry := tools.NewRegistry()

handler := admin.NewAdminHandler(pool, registry, admin.WithAPIToken("secret-token"))

http.ListenAndServe(":8082", handler)
```

## 端点清单

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| GET | `/api/health` | 健康检查 | 否 |
| GET | `/api/agents` | 列出所有 Agent | 是 |
| GET | `/api/agents/{id}` | 查看 Agent 详情 | 是 |
| GET | `/api/tasks` | 列出任务 | 是 |
| GET | `/api/stats` | 统计信息 | 是 |
| GET | `/api/system` | 系统信息 | 是 |
| GET | `/api/tools` | 列出工具 | 是 |
| GET | `/api/tools/{name}` | 工具详情 | 是 |
| GET | `/api/tools/categories` | 工具分类 | 是 |
| GET | `/api/workflows` | 列出工作流 | 是 |
| GET | `/api/workflows/{id}` | 工作流详情 | 是 |
| GET | `/api/logs/stream` | 实时日志流 | 是 |

## 认证

所有管理端点需要在请求头中提供：

```
Authorization: Bearer <token>
```

未配置 `WithAPIToken` 时，管理端点返回 401。

## 示例

```bash
curl -H "Authorization: Bearer secret-token" \
     http://localhost:8082/api/agents
```

## 下一步

- 查看 `cmd/admin/main.go`
- 了解 [检查点持久化](../concepts/状态持久化.md)
