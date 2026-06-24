package otel

import (
	"context"
	"strings"
	"testing"
)

func TestParseBaggage_Empty(t *testing.T) {
	b := ParseBaggage("")
	if b != nil {
		t.Error("empty string should return nil")
	}
}

func TestParseBaggage_SingleItem(t *testing.T) {
	b := ParseBaggage("key1=value1")
	if b == nil {
		t.Fatal("should not be nil")
	}
	item, ok := b.Get("key1")
	if !ok {
		t.Error("should find key1")
	}
	if item.Value != "value1" {
		t.Errorf("expected value1, got %q", item.Value)
	}
}

func TestParseBaggage_MultipleItems(t *testing.T) {
	b := ParseBaggage("key1=value1,key2=value2")
	if b == nil {
		t.Fatal("should not be nil")
	}
	item1, ok1 := b.Get("key1")
	item2, ok2 := b.Get("key2")
	if !ok1 || !ok2 {
		t.Error("should find both keys")
	}
	if item1.Value != "value1" || item2.Value != "value2" {
		t.Errorf("expected value1/value2, got %q/%q", item1.Value, item2.Value)
	}
}

func TestParseBaggage_WithMetadata(t *testing.T) {
	b := ParseBaggage("key1=value1;metadata1,key2=value2;metadata2")
	if b == nil {
		t.Fatal("should not be nil")
	}
	item1, ok1 := b.Get("key1")
	item2, ok2 := b.Get("key2")
	if !ok1 || !ok2 {
		t.Error("should find both keys")
	}
	if item1.Metadata != "metadata1" {
		t.Errorf("expected metadata1, got %q", item1.Metadata)
	}
	if item2.Metadata != "metadata2" {
		t.Errorf("expected metadata2, got %q", item2.Metadata)
	}
}

func TestParseBaggage_Whitespace(t *testing.T) {
	// W3C 规范允许空格
	b := ParseBaggage("  key1 = value1 , key2 = value2  ")
	if b == nil {
		t.Fatal("should not be nil")
	}
	item1, ok1 := b.Get("key1")
	item2, ok2 := b.Get("key2")
	if !ok1 || !ok2 {
		t.Error("should find both keys after trimming whitespace")
	}
	if item1.Value != "value1" || item2.Value != "value2" {
		t.Errorf("expected value1/value2, got %q/%q", item1.Value, item2.Value)
	}
}

func TestParseBaggage_SkipInvalidItems(t *testing.T) {
	// 缺少等号的条目应被跳过
	b := ParseBaggage("key1=value1,invalid,key2=value2")
	if b == nil {
		t.Fatal("should not be nil")
	}
	if _, ok := b.Get("invalid"); ok {
		t.Error("invalid item without '=' should be skipped")
	}
	item1, ok1 := b.Get("key1")
	item2, ok2 := b.Get("key2")
	if !ok1 || !ok2 {
		t.Error("valid items should be parsed")
	}
	if item1.Value != "value1" || item2.Value != "value2" {
		t.Errorf("expected value1/value2, got %q/%q", item1.Value, item2.Value)
	}
}

func TestBaggage_String(t *testing.T) {
	b := ParseBaggage("key1=value1;metadata1,key2=value2")
	s := b.String()
	if !strings.Contains(s, "key1=value1;metadata1") {
		t.Errorf("should contain key1 with metadata, got: %q", s)
	}
	if !strings.Contains(s, "key2=value2") {
		t.Errorf("should contain key2, got: %q", s)
	}
}

func TestBaggage_String_Empty(t *testing.T) {
	b := &Baggage{items: make(map[string]BaggageItem)}
	s := b.String()
	if s != "" {
		t.Errorf("empty baggage should produce empty string, got: %q", s)
	}
}

func TestBaggage_Set(t *testing.T) {
	b := &Baggage{items: make(map[string]BaggageItem)}
	b.Set("key1", "value1")
	item, ok := b.Get("key1")
	if !ok {
		t.Error("should find key1 after Set")
	}
	if item.Value != "value1" {
		t.Errorf("expected value1, got %q", item.Value)
	}
}

func TestBaggage_Delete(t *testing.T) {
	b := ParseBaggage("key1=value1,key2=value2")
	b.Delete("key1")
	if _, ok := b.Get("key1"); ok {
		t.Error("key1 should be deleted")
	}
	if _, ok := b.Get("key2"); !ok {
		t.Error("key2 should still exist")
	}
}

func TestBaggage_Keys(t *testing.T) {
	b := ParseBaggage("alpha=1,beta=2,gamma=3")
	keys := b.Keys()
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
	// 验证所有键都存在
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	for _, expected := range []string{"alpha", "beta", "gamma"} {
		if !keySet[expected] {
			t.Errorf("expected key %q not found", expected)
		}
	}
}

func TestBaggage_InjectExtract(t *testing.T) {
	b := &Baggage{items: map[string]BaggageItem{
		"trace_id": {Value: "abc123", Metadata: "sdk"},
		"user_id":  {Value: "user42"},
	}}

	ctx := b.Inject(context.Background())

	// 从 context 提取
	extracted := Extract(ctx)
	if extracted == nil {
		t.Fatal("extracted baggage should not be nil")
	}

	item1, ok1 := extracted.Get("trace_id")
	item2, ok2 := extracted.Get("user_id")
	if !ok1 || !ok2 {
		t.Error("should find both keys after extract")
	}
	if item1.Value != "abc123" || item1.Metadata != "sdk" {
		t.Errorf("expected abc123/sdk, got %q/%q", item1.Value, item1.Metadata)
	}
	if item2.Value != "user42" {
		t.Errorf("expected user42, got %q", item2.Value)
	}
}

func TestBaggage_Extract_EmptyContext(t *testing.T) {
	b := Extract(context.Background())
	if b != nil {
		t.Error("context without baggage should return nil")
	}
}

func TestBaggage_RoundTrip(t *testing.T) {
	original := "sessionId=abc123;v=1,userId=user42"
	b := ParseBaggage(original)
	serialized := b.String()

	// 重新解析序列化结果
	b2 := ParseBaggage(serialized)
	item1, ok1 := b2.Get("sessionId")
	item2, ok2 := b2.Get("userId")
	if !ok1 || !ok2 {
		t.Error("round-trip should preserve keys")
	}
	if item1.Value != "abc123" {
		t.Errorf("expected abc123, got %q", item1.Value)
	}
	if item1.Metadata != "v=1" {
		t.Errorf("expected metadata 'v=1', got %q", item1.Metadata)
	}
	if item2.Value != "user42" {
		t.Errorf("expected user42, got %q", item2.Value)
	}
}
