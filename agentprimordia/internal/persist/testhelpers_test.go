//go:build etcd || redis

// testhelpers_test.go — 分布式 Checkpoint E2E 测试辅助函数
//
// 提供 etcd/redis 连接检测工具，用于在基础设施不可用时优雅跳过测试。
// 本文件仅在 etcd/redis 集成构建下编译（默认构建下这些符号不参与编译，
// 避免 golangci-lint unused 误报）。
package persist //nolint:unused // 供 build-tag(etcd/redis) 集成测试使用，默认构建下视为未引用

import (
	"net"
	"os"
	"testing"
	"time"
)

// defaultEtcdAddr 默认 etcd 地址
const defaultEtcdAddr = "localhost:2379"

// defaultRedisAddr 默认 redis 地址
const defaultRedisAddr = "localhost:6379"

// getEtcdAddr 获取 etcd 地址（支持环境变量覆盖）
func getEtcdAddr() string {
	if addr := os.Getenv("ETCD_ADDR"); addr != "" {
		return addr
	}
	return defaultEtcdAddr
}

// getRedisAddr 获取 redis 地址（支持环境变量覆盖）
func getRedisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return defaultRedisAddr
}

// isReachable 检测 TCP 端口是否可达
func isReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// requireEtcd 跳过测试（etcd 不可达）
func requireEtcd(t *testing.T) {
	t.Helper()
	addr := getEtcdAddr()
	if !isReachable(addr) {
		t.Skipf("etcd 不可达 (%s)，跳过测试。设置 ETCD_ADDR 环境变量可自定义地址", addr)
	}
}

// requireRedis 跳过测试（redis 不可达）
func requireRedis(t *testing.T) {
	t.Helper()
	addr := getRedisAddr()
	if !isReachable(addr) {
		t.Skipf("redis 不可达 (%s)，跳过测试。设置 REDIS_ADDR 环境变量可自定义地址", addr)
	}
}
