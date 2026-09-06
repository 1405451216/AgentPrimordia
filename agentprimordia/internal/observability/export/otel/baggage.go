package otel

import (
	"context"
	"strings"
)

// contextKey 用于 context 存储 Baggage 的键类型
type contextKey struct{}

// baggageKey Baggage 在 context 中的键
var baggageKey = contextKey{}

// Baggage W3C Baggage 传播器
type Baggage struct {
	items map[string]BaggageItem
}

// BaggageItem Baggage 条目
type BaggageItem struct {
	Value    string
	Metadata string
}

// ParseBaggage 从 W3C Baggage header 解析
// 格式: key1=value1;metadata1,key2=value2
func ParseBaggage(header string) *Baggage {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}

	b := &Baggage{items: make(map[string]BaggageItem)}

	// 按逗号分隔条目
	parts := strings.Split(header, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 分离 metadata（分号后部分）
		var valuePart, metadataPart string
		if idx := strings.Index(part, ";"); idx >= 0 {
			valuePart = strings.TrimSpace(part[:idx])
			metadataPart = strings.TrimSpace(part[idx+1:])
		} else {
			valuePart = part
		}

		// 分离 key=value
		eqIdx := strings.Index(valuePart, "=")
		if eqIdx < 0 {
			// 缺少等号的条目跳过
			continue
		}

		key := strings.TrimSpace(valuePart[:eqIdx])
		value := strings.TrimSpace(valuePart[eqIdx+1:])

		if key == "" {
			continue
		}

		b.items[key] = BaggageItem{
			Value:    value,
			Metadata: metadataPart,
		}
	}

	if len(b.items) == 0 {
		return nil
	}

	return b
}

// String 序列化为 W3C Baggage header 格式
func (b *Baggage) String() string {
	if b == nil || len(b.items) == 0 {
		return ""
	}

	// 按键排序以保证输出稳定性
	keys := make([]string, 0, len(b.items))
	for k := range b.items {
		keys = append(keys, k)
	}

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		item := b.items[k]
		s := k + "=" + item.Value
		if item.Metadata != "" {
			s += ";" + item.Metadata
		}
		parts = append(parts, s)
	}

	return strings.Join(parts, ",")
}

// Get 获取条目
func (b *Baggage) Get(key string) (BaggageItem, bool) {
	if b == nil {
		return BaggageItem{}, false
	}
	item, ok := b.items[key]
	return item, ok
}

// Set 设置条目
func (b *Baggage) Set(key, value string) {
	if b == nil {
		return
	}
	if b.items == nil {
		b.items = make(map[string]BaggageItem)
	}
	b.items[key] = BaggageItem{Value: value}
}

// Delete 删除条目
func (b *Baggage) Delete(key string) {
	if b == nil {
		return
	}
	delete(b.items, key)
}

// Keys 返回所有键
func (b *Baggage) Keys() []string {
	if b == nil {
		return nil
	}
	keys := make([]string, 0, len(b.items))
	for k := range b.items {
		keys = append(keys, k)
	}
	return keys
}

// Inject 向 context 注入 Baggage
func (b *Baggage) Inject(ctx context.Context) context.Context {
	if b == nil {
		return ctx
	}
	return context.WithValue(ctx, baggageKey, b)
}

// Extract 从 context 提取 Baggage
func Extract(ctx context.Context) *Baggage {
	if ctx == nil {
		return nil
	}
	b, _ := ctx.Value(baggageKey).(*Baggage)
	return b
}
