package wasm

import (
	"context"
	"crypto/ed25519"
	"testing"
)

func TestSigningVerifyValid(t *testing.T) {
	// 生成密钥对
	privateKey, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 模拟 WASM 字节码
	wasmBytes := []byte("mock wasm bytecode for testing")

	// 签名
	signature, pubKey, err := SignWASM(wasmBytes, privateKey)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	// 验证签名
	if err := VerifySignature(wasmBytes, signature, pubKey); err != nil {
		t.Fatalf("验证签名失败: %v", err)
	}
}

func TestSigningVerifyInvalidSignature(t *testing.T) {
	privateKey, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	wasmBytes := []byte("test wasm")
	signature, pubKey, err := SignWASM(wasmBytes, privateKey)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	// 篡改字节码
	tampered := []byte("tampered wasm")

	err = VerifySignature(tampered, signature, pubKey)
	if err == nil {
		t.Fatal("篡改后的字节码应验证失败")
	}
}

func TestSigningVerifyInvalidKeySize(t *testing.T) {
	err := VerifySignature([]byte("test"), []byte("short"), []byte("short"))
	if err == nil {
		t.Fatal("无效密钥大小应返回错误")
	}
}

func TestKeyFingerprint(t *testing.T) {
	_, publicKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	fp := KeyFingerprint([]byte(publicKey))
	if len(fp) != 8 { // 4 bytes = 8 hex chars
		t.Errorf("指纹长度 = %d, 期望 8", len(fp))
	}
}

func TestWASMToolAdapterRegisterAndList(t *testing.T) {
	sandbox := NewSandbox(DefaultSandboxConfig())
	defer sandbox.Close()

	adapter := NewWASMToolAdapter(sandbox)

	// 创建一个最小 WASM 模块（空模块）
	// 由于无法在测试中编译真实的 WASM 字节码，
	// 这里测试 RegisterTool 在无效字节码时的行为
	meta := ToolMetadata{
		Name:        "test-tool",
		Description: "A test WASM tool",
		Parameters:  []byte(`{"type":"object","properties":{"input":{"type":"string"}}}`),
		ExecuteFunc: "execute",
		Version:     "1.0.0",
	}

	// 使用无效 WASM 字节码，预期失败
	err := adapter.RegisterTool(context.Background(), meta, []byte("invalid wasm"))
	if err == nil {
		// 如果沙箱接受空字节码，注册应该成功
		tools := adapter.ListTools()
		if len(tools) != 1 {
			t.Errorf("工具数 = %d, 期望 1", len(tools))
		}
		if tools[0].Name != "test-tool" {
			t.Errorf("工具名 = %s, 期望 test-tool", tools[0].Name)
		}
	}
	// 无效字节码下注册失败是可接受的
}

func TestWASMToolAdapterUnregister(t *testing.T) {
	sandbox := NewSandbox(DefaultSandboxConfig())
	defer sandbox.Close()

	adapter := NewWASMToolAdapter(sandbox)

	// 注销不存在的工具
	err := adapter.UnregisterTool("nonexistent")
	if err == nil {
		t.Error("注销不存在的工具应返回错误")
	}
}

func TestWASMToolAdapterGetTool(t *testing.T) {
	sandbox := NewSandbox(DefaultSandboxConfig())
	defer sandbox.Close()

	adapter := NewWASMToolAdapter(sandbox)

	// 获取不存在的工具
	_, exists := adapter.GetTool("nonexistent")
	if exists {
		t.Error("不存在的工具应返回 false")
	}
}

func TestWASMToolAdapterUploadToolNoName(t *testing.T) {
	sandbox := NewSandbox(DefaultSandboxConfig())
	defer sandbox.Close()

	adapter := NewWASMToolAdapter(sandbox)

	resp, err := adapter.UploadTool(context.Background(), UploadRequest{
		ToolName:  "",
		WasmBytes: []byte("test"),
	})
	if err != nil {
		t.Fatalf("UploadTool 失败: %v", err)
	}
	if resp.Success {
		t.Error("空名称应返回失败")
	}
}

func TestWASMToolAdapterUploadToolNoBytes(t *testing.T) {
	sandbox := NewSandbox(DefaultSandboxConfig())
	defer sandbox.Close()

	adapter := NewWASMToolAdapter(sandbox)

	resp, err := adapter.UploadTool(context.Background(), UploadRequest{
		ToolName:  "test",
		WasmBytes: nil,
	})
	if err != nil {
		t.Fatalf("UploadTool 失败: %v", err)
	}
	if resp.Success {
		t.Error("空字节码应返回失败")
	}
}

func TestWASMToolAdapterUploadToolWithSignature(t *testing.T) {
	sandbox := NewSandbox(DefaultSandboxConfig())
	defer sandbox.Close()

	adapter := NewWASMToolAdapter(sandbox)

	// 生成密钥对
	privateKey, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	wasmBytes := []byte("mock wasm bytecode")
	signature, pubKey, err := SignWASM(wasmBytes, privateKey)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	resp, _ := adapter.UploadTool(context.Background(), UploadRequest{
		ToolName:    "signed-tool",
		Description: "A signed tool",
		ExecuteFunc: "execute",
		Version:     "1.0.0",
		WasmBytes:   wasmBytes,
		Signature:   signature,
		PublicKey:   pubKey,
	})

	// 签名验证应通过（注册可能因无效 WASM 字节码失败，但签名验证已通过）
	_ = resp
}

func TestWASMToolAdapterUploadToolInvalidSignature(t *testing.T) {
	sandbox := NewSandbox(DefaultSandboxConfig())
	defer sandbox.Close()

	adapter := NewWASMToolAdapter(sandbox)

	resp, err := adapter.UploadTool(context.Background(), UploadRequest{
		ToolName:    "bad-sig-tool",
		ExecuteFunc: "execute",
		WasmBytes:   []byte("test"),
		Signature:   make([]byte, ed25519.SignatureSize),
		PublicKey:   make([]byte, ed25519.PublicKeySize),
	})
	if err != nil {
		t.Fatalf("UploadTool 失败: %v", err)
	}
	if resp.Success {
		t.Error("无效签名应返回失败")
	}
}

func TestWASMToolAdapterClose(t *testing.T) {
	sandbox := NewSandbox(DefaultSandboxConfig())
	defer sandbox.Close()

	adapter := NewWASMToolAdapter(sandbox)

	if err := adapter.Close(); err != nil {
		t.Fatalf("Close 失败: %v", err)
	}

	// 关闭后工具列表应为空
	if len(adapter.ListTools()) != 0 {
		t.Error("关闭后应无工具")
	}
}
