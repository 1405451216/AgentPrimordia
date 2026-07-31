//go:build e2e

// e2e_chaos_helpers_test.go — 混沌工程 E2E 测试辅助函数
package chaos

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// isPrivilegedContainer 检查当前进程是否运行在特权容器中。
// 通过读取 /proc/self/status 中的 CapEff 字段判断是否具有 CAP_NET_ADMIN（bit 12）。
// 非 Linux 平台始终返回 false。
func isPrivilegedContainer() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "CapEff:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return false
			}
			capEff, err := strconv.ParseUint(fields[1], 16, 64)
			if err != nil {
				return false
			}
			// CAP_NET_ADMIN = 12
			return capEff&(1<<12) != 0
		}
	}
	return false
}

// requireLinuxRoot 跳过测试（非 Linux 或非 root 环境）。
// 用于需要 iptables/tc 等特权操作的测试。
func requireLinuxRoot(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("需要 Linux 环境，当前平台: %s", runtime.GOOS)
	}
	if os.Getuid() != 0 {
		t.Skip("需要 root 权限或 CAP_NET_ADMIN 能力，跳过")
	}
}
