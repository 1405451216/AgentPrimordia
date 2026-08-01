// Command studio 启动 AgentPrimordia Studio 后端 HTTP 服务。
//
// 提供 studio/web 四面板（Chaos Lab / Cluster / Learning / Marketplace）
// 依赖的 /api/v1/* 端点。默认使用内置 demo 数据开箱即用；生产接入真实引擎
// 时，通过 internal/studio 的 With* 选项注入对应服务。
//
// 用法：
//
//	go run ./cmd/studio -addr :8090
//	go run ./cmd/studio -addr :8090 -token <bearer>   # 启用鉴权
//
// Studio Web 开发模式：npm run dev（vite.config.ts 已将 /api 代理到 :8090）。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"agentprimordia/internal/studio"
)

func main() {
	addr := flag.String("addr", ":8090", "Studio 后端监听地址")
	token := flag.String("token", "", "可选：Bearer 访问令牌，设置后保护所有 /api/v1/* 端点")
	flag.Parse()

	handler := studio.NewStudioHandler()

	server := &http.Server{
		Addr:    *addr,
		Handler: withOptionalAuth(handler, *token),
	}

	go func() {
		fmt.Printf("Studio 后端监听于 %s\n", *addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\n正在关闭服务器...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("服务器关闭出错: %v", err)
	}
	fmt.Println("服务器已优雅退出")
}

// withOptionalAuth 在设置了 -token 时对 /api/v1/* 端点做 Bearer 鉴权。
func withOptionalAuth(next http.Handler, token string) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
