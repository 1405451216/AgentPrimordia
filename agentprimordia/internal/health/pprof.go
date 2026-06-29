// Package health 提供 pprof 性能分析端点注册。
// 通过 RegisterPProf 将 /debug/pprof/ 路由注册到任意 http.ServeMux，
// 适用于生产环境运行时性能诊断。
package health

import (
	"net/http"
	"net/http/pprof"
)

// RegisterPProf 将 pprof 端点注册到给定的 http.ServeMux。
// 注册以下路由：
//   - /debug/pprof/            — 索引页（链接到各 profile 类型）
//   - /debug/pprof/cmdline     — 当前进程命令行
//   - /debug/pprof/profile     — CPU profile（30 秒采样）
//   - /debug/pprof/symbol      — 符号表
//   - /debug/pprof/trace       — 执行追踪
//   - /debug/pprof/heap        — 堆分配 profile
//   - /debug/pprof/goroutine   — Goroutine 堆栈
//   - /debug/pprof/threadcreate — 线程创建
//   - /debug/pprof/block       — 阻塞 profile
//   - /debug/pprof/mutex       — 互斥锁 profile
//
// 使用示例：
//
//	mux := http.NewServeMux()
//	health.RegisterPProf(mux)
//	go http.ListenAndServe("localhost:6060", mux)
//
// 安全提示：pprof 端点会暴露进程内部信息，生产环境应仅监听 localhost
// 或通过鉴权中间件保护。
func RegisterPProf(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// 手动注册各 profile 类型（pprof.Handler 返回 http.Handler，需适配 HandleFunc）
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
}

// PProfHandler 返回一个包含 pprof 端点的 http.Handler（独立 ServeMux）。
// 适用于不需要自定义路由、仅暴露 pprof 的场景。
//
// 使用示例：
//
//	go http.ListenAndServe("localhost:6060", health.PProfHandler())
func PProfHandler() http.Handler {
	mux := http.NewServeMux()
	RegisterPProf(mux)
	return mux
}
