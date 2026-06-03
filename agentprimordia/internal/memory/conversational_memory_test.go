package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestConversationalMemory_BasicOperations(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		MaxMessages:    10,
		SummaryTrigger: 8,
	})

	ctx := context.Background()

	err := mem.AddMessage(ctx, "user", "Hello, how are you?", nil)
	if err != nil {
		t.Fatalf("AddMessage error: %v", err)
	}

	err = mem.AddMessage(ctx, "assistant", "I'm doing well, thank you! How can I help you?", nil)
	if err != nil {
		t.Fatalf("AddMessage error: %v", err)
	}

	messages := mem.GetMessages()
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	count := mem.GetMessageCount()
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	totalCount := mem.GetTotalMessageCount()
	if totalCount != 2 {
		t.Errorf("expected total count 2, got %d", totalCount)
	}

	t.Logf("✅ Basic operations: messages=%d total=%d", count, totalCount)
}

func TestConversationalMemory_WindowManagement(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		MaxMessages:    5,
		SummaryTrigger: 4,
	})

	ctx := context.Background()

	for i := 0; i < 8; i++ {
		_ = mem.AddMessage(ctx, "user", fmt.Sprintf("Message %d", i), nil)
		_ = mem.AddMessage(ctx, "assistant", fmt.Sprintf("Response %d", i), nil)
	}

	currentCount := mem.GetMessageCount()
	if currentCount > 5 {
		t.Errorf("expected at most 5 messages in window, got %d", currentCount)
	}

	recent := mem.GetRecentMessages(4)
	if len(recent) > 4+1 { // +1 for possible summary
		t.Errorf("expected at most 5 recent messages (including summary), got %d", len(recent))
	}

	t.Logf("✅ Window management: current=%d total=%d", currentCount, mem.GetTotalMessageCount())
}

func TestConversationalMemory_SummaryCompression(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		MaxMessages:    6,
		SummaryTrigger: 4,
	})

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_ = mem.AddMessage(ctx, "user", fmt.Sprintf("User question about topic %d with detailed explanation", i), nil)
		_ = mem.AddMessage(ctx, "assistant", fmt.Sprintf("Assistant response about topic %d with comprehensive answer and examples", i), nil)
	}

	summary := mem.GetSummary()
	if summary == "" {
		t.Error("expected non-empty summary after compression")
	}

	stats := mem.GetStats()
	compressionCount := stats["compression_count"].(int)
	if compressionCount == 0 {
		t.Error("expected some messages to be compressed")
	}

	t.Logf("✅ Summary compression: summary_len=%d compressed=%d", len(summary), compressionCount)
	t.Logf("   Summary preview: %.100s...", summary)
}

func TestConversationalMemory_ClearAndReset(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{})

	ctx := context.Background()

	_ = mem.AddMessage(ctx, "user", "Test message", nil)
	_ = mem.AddMessage(ctx, "assistant", "Test response", nil)

	mem.Clear()

	if mem.GetMessageCount() != 0 {
		t.Error("expected 0 messages after clear")
	}
	if mem.GetSummary() != "" {
		t.Error("expected empty summary after clear")
	}
	if mem.GetTotalMessageCount() != 0 {
		t.Error("expected 0 total messages after clear")
	}

	t.Logf("✅ Clear and reset successful")
}

func TestConversationalMemory_ExportImport(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{})

	ctx := context.Background()

	_ = mem.AddMessage(ctx, "system", "You are a helpful assistant.", map[string]string{"type": "system"})
	_ = mem.AddMessage(ctx, "user", "Hello!", nil)
	_ = mem.AddMessage(ctx, "assistant", "Hi there!", nil)

	data, err := mem.Export()
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}

	newMem := NewConversationalMemory(ConversationalMemoryConfig{})
	err = newMem.Import(data)
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}

	if newMem.GetMessageCount() != mem.GetMessageCount() {
		t.Errorf("message count mismatch after import: expected %d, got %d",
			mem.GetMessageCount(), newMem.GetMessageCount())
	}

	t.Logf("✅ Export/Import: exported=%d bytes messages=%d", len(data), newMem.GetMessageCount())
}

func TestConversationalMemory_Stats(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		MaxMessages:    20,
		SummaryTrigger: 15,
	})

	ctx := context.Background()

	for i := 0; i < 12; i++ {
		_ = mem.AddMessage(ctx, "user", fmt.Sprintf("User message %d", i), nil)
		if i%2 == 0 {
			_ = mem.AddMessage(ctx, "assistant", fmt.Sprintf("Assistant response %d", i), nil)
		}
	}

	stats := mem.GetStats()

	if stats["current_messages"].(int) <= 0 {
		t.Errorf("unexpected current_messages: %d", stats["current_messages"])
	}
	if stats["total_messages"].(int) != 18 {
		t.Errorf("unexpected total_messages: %d, expected 18", stats["total_messages"])
	}
	userMsgs := stats["user_messages"].(int)
	if userMsgs <= 0 {
		t.Errorf("unexpected user_messages: %d, expected > 0", userMsgs)
	}
	if _, ok := stats["last_updated"]; !ok {
		t.Error("missing last_updated in stats")
	}

	statsJSON, _ := json.MarshalIndent(stats, "", "  ")
	t.Logf("✅ Stats:\n%s", string(statsJSON))
}

func TestConversationalMemory_Metadata(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		Metadata: map[string]string{
			"session_id": "test-session-123",
			"user_id":    "user-456",
		},
	})

	ctx := context.Background()

	_ = mem.AddMessage(ctx, "user", "Test", map[string]string{"source": "web"})

	messages := mem.GetMessages()
	foundMetadata := false
	for _, msg := range messages {
		if msg.Metadata != nil && msg.Metadata["source"] == "web" {
			foundMetadata = true
			break
		}
	}

	if !foundMetadata {
		t.Error("message metadata not preserved")
	}

	t.Logf("✅ Metadata handling working")
}

func TestDefaultCompressor_BasicCompression(t *testing.T) {
	compressor := &DefaultCompressor{}

	messages := []*Message{
		{Role: "user", Content: "I want to learn about machine learning basics"},
		{Role: "assistant", Content: "Machine learning is a subset of AI that enables systems to learn from data"},
		{Role: "user", Content: "What are the main types of ML?"},
		{Role: "assistant", Content: "The main types are supervised, unsupervised, and reinforcement learning"},
	}

	summary, err := compressor.Compress(context.Background(), messages, "")
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	if summary == "" {
		t.Error("expected non-empty summary")
	}
	if !strings.Contains(strings.ToLower(summary), "user") || !strings.Contains(strings.ToLower(summary), "assistant") {
		t.Error("summary should mention both user and assistant content")
	}

	t.Logf("✅ Default compressor: summary=%.150s...", summary)
}

func TestConversationalMemory_HighVolume(t *testing.T) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		MaxMessages:    50,
		SummaryTrigger: 40,
	})

	ctx := context.Background()

	startTime := time.Now()
	for i := 0; i < 200; i++ {
		_ = mem.AddMessage(ctx, "user", fmt.Sprintf("User message number %d with some content", i), nil)
		_ = mem.AddMessage(ctx, "assistant", fmt.Sprintf("Assistant response number %d with helpful information", i), nil)
	}
	duration := time.Since(startTime)

	stats := mem.GetStats()
	currentMsgs := stats["current_messages"].(int)
	totalMsgs := stats["total_messages"].(int)

	if currentMsgs > 50 {
		t.Errorf("window should not exceed 50 messages, got %d", currentMsgs)
	}
	if totalMsgs != 400 {
		t.Errorf("total should be 400, got %d", totalMsgs)
	}

	t.Logf("✅ High volume test: added 400 msgs in %v", duration)
	t.Logf("   Window size: %d, Compressed: %d", currentMsgs, 400-currentMsgs)
}

func BenchmarkConversationalMemory_AddMessage(b *testing.B) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		MaxMessages:    1000,
		SummaryTrigger: 800,
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mem.AddMessage(ctx, "user", fmt.Sprintf("Benchmark message %d", i), nil)
	}
}

func BenchmarkConversationalMemory_GetMessages(b *testing.B) {
	mem := NewConversationalMemory(ConversationalMemoryConfig{
		MaxMessages: 100,
	})

	ctx := context.Background()
	for i := 0; i < 80; i++ {
		_ = mem.AddMessage(ctx, "user", fmt.Sprintf("Message %d", i), nil)
		_ = mem.AddMessage(ctx, "assistant", fmt.Sprintf("Response %d", i), nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mem.GetMessages()
	}
}
