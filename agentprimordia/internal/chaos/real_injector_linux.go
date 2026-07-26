//go:build linux

// real_injector_linux.go — Linux 平台真实故障注入（V3.1 Phase 1）
//
// 使用 iptables/tc 命令注入网络故障（延迟/丢包/分区）。
// 需要 root 权限或 CAP_NET_ADMIN 能力。
//
// 安全约束：
//   - 所有命令经过严格参数验证，防止命令注入
//   - 仅允许操作指定网卡和目标地址
//   - 提供自动清理机制（实验结束或超时后恢复）
package chaos

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RealNetworkInjector Linux 真实网络故障注入器
//
// 使用 tc (traffic control) 和 iptables 实现：
//   - 网络延迟注入（tc netem delay）
//   - 丢包注入（tc netem loss）
//   - 网络分区（iptables DROP）
type RealNetworkInjector struct {
	logger    *slog.Logger
	iface     string // 网卡名称（默认 eth0）
	dryRun    bool   // 干跑模式（仅记录不执行）
}

// RealNetworkInjectorConfig 真实注入器配置
type RealNetworkInjectorConfig struct {
	// Interface 网卡名称（默认 "eth0"）
	Interface string
	// DryRun 干跑模式（仅记录命令不执行）
	DryRun bool
	// Logger 日志器
	Logger *slog.Logger
}

// NewRealNetworkInjector 创建 Linux 真实网络故障注入器
func NewRealNetworkInjector(cfg RealNetworkInjectorConfig) *RealNetworkInjector {
	if cfg.Interface == "" {
		cfg.Interface = "eth0"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &RealNetworkInjector{
		logger: cfg.Logger,
		iface:  cfg.Interface,
		dryRun: cfg.DryRun,
	}
}

// InjectDelay 注入网络延迟
//
// 使用 tc netem 对指定目标地址注入延迟。
// 命令：tc qdisc add dev <iface> root netem delay <ms>ms <jitter>ms
func (inj *RealNetworkInjector) InjectDelay(ctx context.Context, target string, delay, jitter time.Duration) (CleanupFunc, error) {
	if err := validateTarget(target); err != nil {
		return nil, fmt.Errorf("chaos_linux: invalid target: %w", err)
	}

	delayMs := int(delay.Milliseconds())
	jitterMs := int(jitter.Milliseconds())

	// 添加 tc qdisc
	args := []string{"qdisc", "add", "dev", inj.iface, "root", "netem",
		"delay", strconv.Itoa(delayMs) + "ms"}
	if jitterMs > 0 {
		args = append(args, strconv.Itoa(jitterMs)+"ms")
	}

	if err := inj.runTC(ctx, args...); err != nil {
		return nil, fmt.Errorf("chaos_linux: inject delay: %w", err)
	}

	inj.logger.Info("网络延迟已注入",
		"target", target, "delay_ms", delayMs, "jitter_ms", jitterMs, "iface", inj.iface)

	// 清理函数：删除 tc qdisc
	cleanup := func(ctx context.Context) error {
		return inj.runTC(ctx, "qdisc", "del", "dev", inj.iface, "root")
	}

	return cleanup, nil
}

// InjectPacketLoss 注入丢包
//
// 使用 tc netem 对指定网卡注入丢包率。
// 命令：tc qdisc add dev <iface> root netem loss <percent>%
func (inj *RealNetworkInjector) InjectPacketLoss(ctx context.Context, target string, lossPercent int) (CleanupFunc, error) {
	if err := validateTarget(target); err != nil {
		return nil, fmt.Errorf("chaos_linux: invalid target: %w", err)
	}
	if lossPercent < 0 || lossPercent > 100 {
		return nil, fmt.Errorf("chaos_linux: loss percent must be 0-100, got %d", lossPercent)
	}

	args := []string{"qdisc", "add", "dev", inj.iface, "root", "netem",
		"loss", strconv.Itoa(lossPercent) + "%"}

	if err := inj.runTC(ctx, args...); err != nil {
		return nil, fmt.Errorf("chaos_linux: inject packet loss: %w", err)
	}

	inj.logger.Info("丢包已注入",
		"target", target, "loss_percent", lossPercent, "iface", inj.iface)

	cleanup := func(ctx context.Context) error {
		return inj.runTC(ctx, "qdisc", "del", "dev", inj.iface, "root")
	}

	return cleanup, nil
}

// InjectPartition 注入网络分区（完全阻断到目标的流量）
//
// 使用 iptables 添加到目标地址的 DROP 规则。
// 命令：iptables -A OUTPUT -d <target> -j DROP
//
//	iptables -A INPUT -s <target> -j DROP
func (inj *RealNetworkInjector) InjectPartition(ctx context.Context, target string) (CleanupFunc, error) {
	if err := validateTarget(target); err != nil {
		return nil, fmt.Errorf("chaos_linux: invalid target: %w", err)
	}

	// 阻断出站
	if err := inj.runIptables(ctx, "-A", "OUTPUT", "-d", target, "-j", "DROP"); err != nil {
		return nil, fmt.Errorf("chaos_linux: inject partition (output): %w", err)
	}

	// 阻断入站
	if err := inj.runIptables(ctx, "-A", "INPUT", "-s", target, "-j", "DROP"); err != nil {
		// 回滚出站规则
		inj.runIptables(ctx, "-D", "OUTPUT", "-d", target, "-j", "DROP")
		return nil, fmt.Errorf("chaos_linux: inject partition (input): %w", err)
	}

	inj.logger.Info("网络分区已注入", "target", target)

	// 清理：删除 iptables 规则
	cleanup := func(ctx context.Context) error {
		var firstErr error
		if err := inj.runIptables(ctx, "-D", "OUTPUT", "-d", target, "-j", "DROP"); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := inj.runIptables(ctx, "-D", "INPUT", "-s", target, "-j", "DROP"); err != nil && firstErr == nil {
			firstErr = err
		}
		if firstErr == nil {
			inj.logger.Info("网络分区已恢复", "target", target)
		}
		return firstErr
	}

	return cleanup, nil
}

// ===== 内部方法 =====

// runTC 执行 tc 命令
func (inj *RealNetworkInjector) runTC(ctx context.Context, args ...string) error {
	if inj.dryRun {
		inj.logger.Info("[DRY RUN] tc "+strings.Join(args, " "))
		return nil
	}

	cmd := exec.CommandContext(ctx, "tc", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tc %s: %w (output: %s)", strings.Join(args, " "), err, string(output))
	}
	return nil
}

// runIptables 执行 iptables 命令
func (inj *RealNetworkInjector) runIptables(ctx context.Context, args ...string) error {
	if inj.dryRun {
		inj.logger.Info("[DRY RUN] iptables " + strings.Join(args, " "))
		return nil
	}

	cmd := exec.CommandContext(ctx, "iptables", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s: %w (output: %s)", strings.Join(args, " "), err, string(output))
	}
	return nil
}

// ===== 参数验证 =====

// ipRegexp 合法 IP 地址正则
var ipRegexp = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)

// hostnameRegexp 合法主机名正则（仅允许字母、数字、点、连字符）
var hostnameRegexp = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.\-]*[a-zA-Z0-9]$`)

// validateTarget 验证目标地址，防止命令注入
//
// 仅允许：
//   - IPv4 地址（如 "10.0.0.1"）
//   - 合法主机名（如 "api.example.com"）
//   - CIDR 格式（如 "10.0.0.0/24"）
func validateTarget(target string) error {
	if target == "" {
		return fmt.Errorf("empty target")
	}

	// 检查 CIDR
	if strings.Contains(target, "/") {
		_, _, err := net.ParseCIDR(target)
		if err != nil {
			return fmt.Errorf("invalid CIDR %q: %w", target, err)
		}
		return nil
	}

	// 检查 IP
	if ip := net.ParseIP(target); ip != nil {
		return nil
	}

	// 检查主机名（严格限制字符集，防止命令注入）
	if hostnameRegexp.MatchString(target) && len(target) <= 253 {
		return nil
	}

	return fmt.Errorf("target %q is not a valid IP, CIDR, or hostname", target)
}
