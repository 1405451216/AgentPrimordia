package memory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteStore_RecordToolUse(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	err = store.RecordToolUse(ctx, "sess1", "agent1", "ReadFile", `{"path":"/tmp/a.txt"}`, "文件内容")
	if err != nil {
		t.Fatalf("RecordToolUse 失败: %v", err)
	}

	count, err := store.Count(ctx, "sess1")
	if err != nil {
		t.Fatalf("Count 失败: %v", err)
	}
	if count != 1 {
		t.Errorf("期望 count=1, 实际 count=%d", count)
	}

	episodes, err := store.List(ctx, &ListOptions{SessionID: "sess1", Limit: 10})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(episodes) != 1 {
		t.Fatalf("期望 1 条记录, 实际 %d 条", len(episodes))
	}

	ep := episodes[0]
	if ep.Role != "tool_use" {
		t.Errorf("期望 role=tool_use, 实际 role=%s", ep.Role)
	}
	if ep.Content != `{"path":"/tmp/a.txt"}` {
		t.Errorf("期望 content 为工具参数, 实际 content=%s", ep.Content)
	}
	if ep.Topics != "ReadFile" {
		t.Errorf("期望 topics=ReadFile, 实际 topics=%s", ep.Topics)
	}
	if ep.Importance != 0.3 {
		t.Errorf("期望 importance=0.3, 实际 importance=%.2f", ep.Importance)
	}
	if ep.Metadata["tool_name"] != "ReadFile" {
		t.Errorf("期望 metadata.tool_name=ReadFile, 实际 %s", ep.Metadata["tool_name"])
	}
	if ep.Metadata["result"] != "文件内容" {
		t.Errorf("期望 metadata.result=文件内容, 实际 %s", ep.Metadata["result"])
	}
	if ep.Metadata["agent_name"] != "agent1" {
		t.Errorf("期望 metadata.agent_name=agent1, 实际 %s", ep.Metadata["agent_name"])
	}
}

func TestSQLiteStore_RecordToolUse_WithSession(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	err = store.RecordToolUse(ctx, "sess1", "agent1", "ReadFile", `{"path":"/a"}`, "ok")
	if err != nil {
		t.Fatalf("RecordToolUse sess1 失败: %v", err)
	}
	err = store.RecordToolUse(ctx, "sess2", "agent2", "WriteFile", `{"path":"/b"}`, "done")
	if err != nil {
		t.Fatalf("RecordToolUse sess2 失败: %v", err)
	}

	count1, _ := store.Count(ctx, "sess1")
	count2, _ := store.Count(ctx, "sess2")
	if count1 != 1 {
		t.Errorf("sess1 期望 count=1, 实际 count=%d", count1)
	}
	if count2 != 1 {
		t.Errorf("sess2 期望 count=1, 实际 count=%d", count2)
	}
}

func TestSQLiteStore_ClearAll_Entire(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	ep1 := MustEpisode("sess1", "user", "hello")
	ep2 := MustEpisode("sess2", "assistant", "world")
	_ = store.Add(ctx, ep1)
	_ = store.Add(ctx, ep2)

	err = store.ClearAll(ctx, "")
	if err != nil {
		t.Fatalf("ClearAll 失败: %v", err)
	}

	total, _ := store.Count(ctx, "")
	if total != 0 {
		t.Errorf("清空后期望 count=0, 实际 count=%d", total)
	}
}

func TestSQLiteStore_ClearAll_BySession(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	ep1 := MustEpisode("sess1", "user", "hello")
	ep2 := MustEpisode("sess2", "assistant", "world")
	ep3 := MustEpisode("sess1", "assistant", "hi")
	_ = store.Add(ctx, ep1)
	_ = store.Add(ctx, ep2)
	_ = store.Add(ctx, ep3)

	err = store.ClearAll(ctx, "sess1")
	if err != nil {
		t.Fatalf("ClearAll 失败: %v", err)
	}

	count1, _ := store.Count(ctx, "sess1")
	count2, _ := store.Count(ctx, "sess2")
	if count1 != 0 {
		t.Errorf("sess1 清空后期望 count=0, 实际 count=%d", count1)
	}
	if count2 != 1 {
		t.Errorf("sess2 期望 count=1, 实际 count=%d", count2)
	}
}

func TestSQLiteStore_ExportMemories_JSON(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	ep1 := MustEpisode("sess1", "user", "hello")
	ep1.Summary = "打招呼"
	ep1.Topics = "greeting"
	_ = store.Add(ctx, ep1)

	data, err := store.ExportMemories(ctx, "", "json")
	if err != nil {
		t.Fatalf("ExportMemories JSON 失败: %v", err)
	}

	var episodes []Episode
	if err := json.Unmarshal(data, &episodes); err != nil {
		t.Fatalf("反序列化 JSON 失败: %v", err)
	}
	if len(episodes) != 1 {
		t.Fatalf("期望 1 条记录, 实际 %d 条", len(episodes))
	}
	if episodes[0].Content != "hello" {
		t.Errorf("期望 content=hello, 实际 content=%s", episodes[0].Content)
	}
	if episodes[0].Summary != "打招呼" {
		t.Errorf("期望 summary=打招呼, 实际 summary=%s", episodes[0].Summary)
	}
}

func TestSQLiteStore_ExportMemories_Markdown(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	ep1 := MustEpisode("sess1", "user", "hello world")
	ep1.Summary = "打招呼"
	ep1.Topics = "greeting"
	_ = store.Add(ctx, ep1)

	data, err := store.ExportMemories(ctx, "", "markdown")
	if err != nil {
		t.Fatalf("ExportMemories Markdown 失败: %v", err)
	}

	md := string(data)
	if !strings.Contains(md, "# 记忆导出") {
		t.Error("Markdown 导出缺少标题")
	}
	if !strings.Contains(md, "hello world") {
		t.Error("Markdown 导出缺少内容")
	}
	if !strings.Contains(md, "打招呼") {
		t.Error("Markdown 导出缺少摘要")
	}
	if !strings.Contains(md, "greeting") {
		t.Error("Markdown 导出缺少标签")
	}
}

func TestSQLiteStore_ExportMemories_BySession(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	ep1 := MustEpisode("sess1", "user", "hello")
	ep2 := MustEpisode("sess2", "assistant", "world")
	_ = store.Add(ctx, ep1)
	_ = store.Add(ctx, ep2)

	data, err := store.ExportMemories(ctx, "sess1", "json")
	if err != nil {
		t.Fatalf("ExportMemories 按会话导出失败: %v", err)
	}

	var episodes []Episode
	if err := json.Unmarshal(data, &episodes); err != nil {
		t.Fatalf("反序列化 JSON 失败: %v", err)
	}
	if len(episodes) != 1 {
		t.Fatalf("期望 1 条记录, 实际 %d 条", len(episodes))
	}
	if episodes[0].SessionID != "sess1" {
		t.Errorf("期望 session_id=sess1, 实际 session_id=%s", episodes[0].SessionID)
	}
}

func TestSQLiteStore_ImportMemories_JSON(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	ep1 := MustEpisode("sess1", "user", "imported content")
	ep1.Summary = "导入测试"
	ep1.Topics = "test"
	ep1.Importance = 0.8

	jsonData, _ := json.Marshal([]*Episode{ep1})

	count, err := store.ImportMemories(ctx, jsonData, "json")
	if err != nil {
		t.Fatalf("ImportMemories 失败: %v", err)
	}
	if count != 1 {
		t.Errorf("期望导入 1 条, 实际导入 %d 条", count)
	}

	episodes, _ := store.List(ctx, &ListOptions{SessionID: "sess1", Limit: 10})
	if len(episodes) != 1 {
		t.Fatalf("期望 1 条记录, 实际 %d 条", len(episodes))
	}
	if episodes[0].Content != "imported content" {
		t.Errorf("期望 content=imported content, 实际 content=%s", episodes[0].Content)
	}
	if episodes[0].Summary != "导入测试" {
		t.Errorf("期望 summary=导入测试, 实际 summary=%s", episodes[0].Summary)
	}
}

func TestSQLiteStore_ImportMemories_RoundTrip(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	ep1 := MustEpisode("sess1", "user", "round trip content")
	ep1.Summary = "往返测试"
	ep1.Topics = "roundtrip"
	ep1.Importance = 0.7
	ep1.Metadata = map[string]string{"key": "value"}
	_ = store.Add(ctx, ep1)

	ep2 := MustEpisode("sess1", "assistant", "response")
	ep2.Summary = "响应"
	_ = store.Add(ctx, ep2)

	exportData, err := store.ExportMemories(ctx, "", "json")
	if err != nil {
		t.Fatalf("ExportMemories 失败: %v", err)
	}

	store2, err := NewSQLiteStore(filepath.Join(t.TempDir(), "roundtrip.db"))
	if err != nil {
		t.Fatalf("创建第二个数据库失败: %v", err)
	}
	defer store2.Close()

	count, err := store2.ImportMemories(ctx, exportData, "json")
	if err != nil {
		t.Fatalf("ImportMemories 失败: %v", err)
	}
	if count != 2 {
		t.Errorf("期望导入 2 条, 实际导入 %d 条", count)
	}

	total, _ := store2.Count(ctx, "")
	if total != 2 {
		t.Errorf("期望 total=2, 实际 total=%d", total)
	}

	episodes, _ := store2.List(ctx, &ListOptions{SessionID: "sess1", Limit: 10})
	if len(episodes) != 2 {
		t.Fatalf("期望 2 条记录, 实际 %d 条", len(episodes))
	}

	found := false
	for _, ep := range episodes {
		if ep.Content == "round trip content" {
			found = true
			if ep.Summary != "往返测试" {
				t.Errorf("期望 summary=往返测试, 实际 summary=%s", ep.Summary)
			}
			if ep.Topics != "roundtrip" {
				t.Errorf("期望 topics=roundtrip, 实际 topics=%s", ep.Topics)
			}
			if ep.Importance != 0.7 {
				t.Errorf("期望 importance=0.7, 实际 importance=%.2f", ep.Importance)
			}
			if ep.Metadata["key"] != "value" {
				t.Errorf("期望 metadata.key=value, 实际 %s", ep.Metadata["key"])
			}
		}
	}
	if !found {
		t.Error("未找到往返测试的记录")
	}
}
