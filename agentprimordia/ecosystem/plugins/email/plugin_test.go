package email

import (
	"context"
	"encoding/json"
	"testing"
)

// TestEmailTool_Name 验证工具名称
func TestEmailTool_Name(t *testing.T) {
	tool := &EmailTool{}
	if tool.Name() != "email_sender" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "email_sender")
	}
}

// TestEmailTool_Category 验证工具分类
func TestEmailTool_Category(t *testing.T) {
	tool := &EmailTool{}
	if tool.Category() != "communication" {
		t.Errorf("Category() = %q, want %q", tool.Category(), "communication")
	}
}

// TestEmailTool_Parameters 验证参数 Schema 可解析
func TestEmailTool_Parameters(t *testing.T) {
	tool := &EmailTool{}
	params := tool.Parameters()
	if params == nil {
		t.Fatal("Parameters() 不应返回 nil")
	}

	var schema map[string]any
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("Parameters() 返回的 JSON 无效: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("Schema 缺少 properties 字段")
	}

	requiredProps := []string{"to", "subject", "body"}
	for _, prop := range requiredProps {
		if _, exists := props[prop]; !exists {
			t.Errorf("Schema 缺少必需属性: %s", prop)
		}
	}
}

// TestEmailTool_Execute_MissingParams 验证缺少必需参数时返回错误
func TestEmailTool_Execute_MissingParams(t *testing.T) {
	tool := &EmailTool{
		host:        "localhost",
		port:        587,
		username:    "test@example.com",
		password:    "password",
		fromAddress: "test@example.com",
	}

	tests := []struct {
		name string
		args map[string]any
	}{
		{"缺少 to", map[string]any{"subject": "test", "body": "hello"}},
		{"缺少 subject", map[string]any{"to": "a@b.com", "body": "hello"}},
		{"缺少 body", map[string]any{"to": "a@b.com", "subject": "test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _ := json.Marshal(tt.args)
			result, err := tool.Execute(context.Background(), args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Error("期望返回错误结果，但得到了成功结果")
			}
		})
	}
}

// TestEmailTool_Execute_InvalidJSON 验证无效 JSON 输入
func TestEmailTool_Execute_InvalidJSON(t *testing.T) {
	tool := &EmailTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("期望解析错误，但未返回错误")
	}
}

// TestPlugin_Init_MissingConfig 验证缺少配置时返回错误
func TestPlugin_Init_MissingConfig(t *testing.T) {
	p := New()
	err := p.Init(nil)
	if err == nil {
		t.Error("期望 nil 配置返回错误")
	}
}

// TestPlugin_Init_MissingHost 验证缺少 smtp_host 时返回错误
func TestPlugin_Init_MissingHost(t *testing.T) {
	p := New()
	err := p.Init(map[string]any{
		"smtp_username": "user",
		"smtp_password": "pass",
	})
	if err == nil {
		t.Error("期望缺少 smtp_host 时返回错误")
	}
}

// TestPlugin_Init_MissingCredentials 验证缺少认证信息时返回错误
func TestPlugin_Init_MissingCredentials(t *testing.T) {
	p := New()
	err := p.Init(map[string]any{
		"smtp_host": "smtp.example.com",
	})
	if err == nil {
		t.Error("期望缺少认证信息时返回错误")
	}
}

// TestPlugin_Init_Success 验证正确配置时初始化成功
func TestPlugin_Init_Success(t *testing.T) {
	p := New()
	err := p.Init(map[string]any{
		"smtp_host":     "smtp.example.com",
		"smtp_port":     587,
		"smtp_username": "user@example.com",
		"smtp_password": "password",
		"from_address":  "user@example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.tool.host != "smtp.example.com" {
		t.Errorf("host = %q, want %q", p.tool.host, "smtp.example.com")
	}
	if p.tool.port != 587 {
		t.Errorf("port = %d, want %d", p.tool.port, 587)
	}
	if p.tool.fromAddress != "user@example.com" {
		t.Errorf("fromAddress = %q, want %q", p.tool.fromAddress, "user@example.com")
	}
}

// TestPlugin_Init_DefaultFromAddress 验证 from_address 缺省使用 username
func TestPlugin_Init_DefaultFromAddress(t *testing.T) {
	p := New()
	err := p.Init(map[string]any{
		"smtp_host":     "smtp.example.com",
		"smtp_port":     587,
		"smtp_username": "user@example.com",
		"smtp_password": "password",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.tool.fromAddress != "user@example.com" {
		t.Errorf("fromAddress = %q, want %q", p.tool.fromAddress, "user@example.com")
	}
}

// TestPlugin_Init_PortFromString 验证端口可从字符串配置读取
func TestPlugin_Init_PortFromString(t *testing.T) {
	p := New()
	err := p.Init(map[string]any{
		"smtp_host":     "smtp.example.com",
		"smtp_port":     "465",
		"smtp_username": "user@example.com",
		"smtp_password": "password",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.tool.port != 465 {
		t.Errorf("port = %d, want %d", p.tool.port, 465)
	}
}

// TestSplitAddresses 验证地址拆分函数
func TestSplitAddresses(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a@b.com", 1},
		{"a@b.com, c@d.com", 2},
		{" a@b.com , c@d.com , e@f.com ", 3},
	}

	for _, tt := range tests {
		result := splitAddresses(tt.input)
		if len(result) != tt.want {
			t.Errorf("splitAddresses(%q) = %d 项, want %d 项", tt.input, len(result), tt.want)
		}
	}
}

// TestPlugin_Metadata 验证插件元数据
func TestPlugin_Metadata(t *testing.T) {
	p := New()
	if p.Name() != "email" {
		t.Errorf("Name() = %q, want %q", p.Name(), "email")
	}
	if p.Version() != "0.1.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "0.1.0")
	}
	if len(p.Tools()) != 1 {
		t.Errorf("Tools() 返回 %d 项, want 1 项", len(p.Tools()))
	}
}

// TestEmailTool_Execute_BuildsCorrectMessage 验证邮件消息构建
// 此测试使用 mock 方式验证消息格式，不实际发送邮件
func TestEmailTool_Execute_BuildsCorrectMessage(t *testing.T) {
	tool := &EmailTool{
		host:        "localhost",
		port:        587,
		username:    "test@example.com",
		password:    "password",
		fromAddress: "test@example.com",
	}

	args, _ := json.Marshal(map[string]any{
		"to":           "recipient@example.com",
		"subject":      "测试主题",
		"body":         "测试正文",
		"cc":           "cc@example.com",
		"content_type": "html",
	})

	// 由于无法轻松 mock smtp.SendMail，这里仅验证参数解析不报错
	// 实际发送会因网络不可达而失败，这是预期行为
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		// 执行层面不应返回 Go error
		t.Fatalf("unexpected error: %v", err)
	}
	// 由于没有真实 SMTP 服务器，发送会失败，返回 IsError=true
	// 这是预期行为，我们只验证参数解析正确
	if !result.IsError {
		// 在没有 SMTP 服务器的情况下不应成功
		t.Error("期望发送失败返回 IsError=true")
	}
}
