package builtin

import (
	"context"
	"encoding/json"
	"testing"
)

// BenchmarkTools_Filesystem_Read 测试文件系统读取性能
func BenchmarkTools_Filesystem_Read(b *testing.B) {
	b.ReportAllocs()

	dir := b.TempDir()

	fs, err := NewFileSystem(dir)
	if err != nil {
		b.Fatalf("NewFileSystem() error: %v", err)
	}

	// 预创建测试文件
	content := make([]byte, 4096)
	for i := range content {
		content[i] = 'A' + byte(i%26)
	}
	writeArgs, _ := json.Marshal(map[string]any{
		"action":  "write",
		"path":    "bench_read.txt",
		"content": string(content),
	})
	_, err = fs.Execute(context.Background(), writeArgs)
	if err != nil {
		b.Fatalf("write setup error: %v", err)
	}

	readArgs, _ := json.Marshal(map[string]any{
		"action": "read",
		"path":   "bench_read.txt",
	})

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := fs.Execute(ctx, readArgs)
		if err != nil {
			b.Fatalf("Execute() error: %v", err)
		}
	}
}

// BenchmarkTools_Filesystem_Write 测试文件系统写入性能
func BenchmarkTools_Filesystem_Write(b *testing.B) {
	b.ReportAllocs()

	dir := b.TempDir()

	fs, err := NewFileSystem(dir)
	if err != nil {
		b.Fatalf("NewFileSystem() error: %v", err)
	}

	writeContent := string(make([]byte, 1024))
	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		args, _ := json.Marshal(map[string]any{
			"action":  "write",
			"path":    "bench_write.txt",
			"content": writeContent,
		})
		_, err := fs.Execute(ctx, args)
		if err != nil {
			b.Fatalf("Execute() error: %v", err)
		}
	}
}

// BenchmarkTools_Shell_Execute 测试 Shell 命令执行性能
func BenchmarkTools_Shell_Execute(b *testing.B) {
	b.ReportAllocs()

	shell := NewShell()

	args, _ := json.Marshal(map[string]any{
		"action":  "execute",
		"command": "echo hello",
	})

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := shell.Execute(ctx, args)
		if err != nil {
			b.Fatalf("Execute() error: %v", err)
		}
	}
}
