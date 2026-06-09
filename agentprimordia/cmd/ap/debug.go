package main

import (
	"fmt"
	"os"

	"agentprimordia/internal/debugger"
)

func runDebug(args []string) {
	var port string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port", "-p":
			i++
			if i >= len(args) {
				errorf("--port requires a value")
				os.Exit(1)
			}
			port = args[i]
		case "--help", "-h":
			fmt.Print(`ap debug — start debug server

用法:
  ap debug [--port 8080]

选项:
  --port, -p   debug server port (default: 6060)

说明:
  Start HTTP debug server to view agent events,
  memory snapshots and runtime status in the browser.

示例:
  ap debug
  ap debug --port 3000
`)
			return
		}
	}
	if port == "" {
		port = "6060"
	}

	// 检查项目目录
	if _, err := findProjectDir(); err != nil {
		warnf("project directory not found, debug server starts with limited features")
	}

	addr := "localhost:" + port
	server := debugger.NewDebugServer(addr)

	successf("调试服务器启动: http://%s", addr)
	infof("run %s in another terminal to start the agent", bold("ap run"))
	infof("press Ctrl+C to stop")
	fmt.Println()

	if err := server.Start(); err != nil {
		errorf("server start failed: %v", err)
		os.Exit(1)
	}

	// 阻塞主线程
	select {}
}
