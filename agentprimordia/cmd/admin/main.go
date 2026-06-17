package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agentprimordia/internal/admin"
	"agentprimordia/internal/pool"
	"agentprimordia/internal/tools"
)

func main() {
	addr := flag.String("addr", ":8080", "管理面板监听地址")
	flag.Parse()

	poolCfg := pool.PoolConfig{MaxConcurrency: 10}
	p := pool.NewPool(poolCfg)
	defer p.Close()

	registry := tools.NewRegistry()
	handler := admin.NewAdminHandler(p, registry)

	server := &http.Server{
		Addr:    *addr,
		Handler: handler,
	}

	// 启动 HTTP 服务器（非阻塞）
	go func() {
		fmt.Printf("管理面板监听于 %s\n", *addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\n正在关闭服务器...")

	// 给予 10 秒时间处理剩余请求
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("服务器关闭出错: %v", err)
	}

	fmt.Println("服务器已优雅退出")
}
