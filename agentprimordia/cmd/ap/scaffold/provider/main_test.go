package main

import (
	"context"
	"strings"
	"testing"
)

func TestProvider_Name(t *testing.T) {
	p := NewProvider(Config{APIKey: "test"})
	if p.Name() == "" {
		t.Fatal("Provider 名称不能为空")
	}
}

func TestNewProvider_Defaults(t *testing.T) {
	p := NewProvider(Config{})
	if p.cfg.Endpoint == "" {
		t.Fatal("Endpoint 应有默认值")
	}
	if p.cfg.Model == "" {
		t.Fatal("Model 应有默认值")
	}
	if p.cfg.Timeout == 0 {
		t.Fatal("Timeout 应有默认值")
	}
}

func TestProvider_Chat_EmptyAPIKey(t *testing.T) {
	p := NewProvider(Config{})
	_, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("API Key 为空时应返回错误")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("错误信息应提及 api_key，实际: %v", err)
	}
}

func TestProvider_Chat_EmptyMessages(t *testing.T) {
	p := NewProvider(Config{APIKey: "test"})
	_, err := p.Chat(context.Background(), ChatRequest{Messages: nil})
	if err == nil {
		t.Fatal("Messages 为空时应返回错误")
	}
	if !strings.Contains(err.Error(), "messages") {
		t.Fatalf("错误信息应提及 messages，实际: %v", err)
	}
}

func TestProvider_Chat_NotImplemented(t *testing.T) {
	p := NewProvider(Config{APIKey: "test"})
	_, err := p.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("占位实现应返回 ErrNotImplemented")
	}
	if err != ErrNotImplemented {
		t.Fatalf("期望 ErrNotImplemented，实际: %v", err)
	}
}

func TestProvider_Close(t *testing.T) {
	p := NewProvider(Config{APIKey: "test"})
	if err := p.Close(); err != nil {
		t.Fatalf("Close 返回错误: %v", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Endpoint == "" || cfg.Model == "" || cfg.Timeout == 0 {
		t.Fatal("DefaultConfig 缺字段")
	}
}
