// Package health 提供 pprof 性能分析端点注册。
// 通过 RegisterPProf 将 /debug/pprof/ 路由注册到任意 http.ServeMux，
// 适用于生产环境运行时性能诊断。
package health

import (
	"errors"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
)

// ErrPProfTokenRequired 当生产模式下 PPROF_TOKEN 未设置时返回此错误。
var ErrPProfTokenRequired = errors.New("health: PPROF_TOKEN environment variable is required in production mode")

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
// Deprecated: 无鉴权版本，生产环境请使用 RegisterPProfSecure 或 RegisterPProfStrict。
// Removed in v4.0.0.
// 仅适用于本地开发调试。
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

// pprofAuthMiddleware 返回一个 Bearer Token 鉴权中间件。
// 从环境变量 PPROF_TOKEN 读取预期 token，若未配置则允许所有请求（开发模式）。
// 生产环境必须设置 PPROF_TOKEN。
func pprofAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := os.Getenv("PPROF_TOKEN")
		if expected == "" {
			// 未配置 token，放行（开发模式）
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
			return
		}
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			http.Error(w, `{"error":"invalid Authorization format, expected Bearer token"}`, http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, prefix)
		if token != expected {
			http.Error(w, `{"error":"invalid token"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RegisterPProfSecure 将 pprof 端点注册到给定的 http.ServeMux，
// 并通过 Bearer Token 鉴权保护。
//
// 鉴权逻辑：
//   - 若环境变量 PPROF_TOKEN 未设置，等同于 RegisterPProf（无鉴权，适用于开发环境）。
//   - 若 PPROF_TOKEN 已设置，请求必须携带 Authorization: Bearer <token> 头。
//
// 使用示例：
//
//	mux := http.NewServeMux()
//	health.RegisterPProfSecure(mux)
//	go http.ListenAndServe("localhost:6060", mux)
func RegisterPProfSecure(mux *http.ServeMux) {
	// 用鉴权中间件包装独立的 pprof 子 mux
	pprofMux := http.NewServeMux()
	pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
	pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	pprofMux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	pprofMux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	pprofMux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	pprofMux.Handle("/debug/pprof/block", pprof.Handler("block"))
	pprofMux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))

	mux.Handle("/debug/pprof/", pprofAuthMiddleware(pprofMux))
}

// PProfHandlerSecure 返回一个包含 pprof 端点的 http.Handler（独立 ServeMux），
// 并启用 Bearer Token 鉴权。
//
// 使用示例：
//
//	go http.ListenAndServe("localhost:6060", health.PProfHandlerSecure())
func PProfHandlerSecure() http.Handler {
	mux := http.NewServeMux()
	RegisterPProfSecure(mux)
	return mux
}

// RegisterPProfStrict 将 pprof 端点注册到给定的 http.ServeMux，
// 强制要求环境变量 PPROF_TOKEN 已设置，否则返回 ErrPProfTokenRequired。
// 与 RegisterPProfSecure 不同，此函数不允许开发模式回退，
// 适用于生产环境配置校验（fail-fast）。
//
// 使用示例：
//
//	mux := http.NewServeMux()
//	if err := health.RegisterPProfStrict(mux); err != nil {
//		log.Fatal("pprof 配置错误: ", err)
//	}
//	go http.ListenAndServe(":6060", mux)
func RegisterPProfStrict(mux *http.ServeMux) error {
	if os.Getenv("PPROF_TOKEN") == "" {
		return ErrPProfTokenRequired
	}
	RegisterPProfSecure(mux)
	return nil
}

// PProfHandlerStrict 返回一个包含 pprof 端点的 http.Handler，
// 强制要求 PPROF_TOKEN 已设置，否则返回错误。
// 生产环境推荐使用此版本。
func PProfHandlerStrict() (http.Handler, error) {
	if os.Getenv("PPROF_TOKEN") == "" {
		return nil, ErrPProfTokenRequired
	}
	return PProfHandlerSecure(), nil
}
