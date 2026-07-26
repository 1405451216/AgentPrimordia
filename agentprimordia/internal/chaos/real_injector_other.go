//go:build !linux

// real_injector_other.go — 非 Linux 平台故障注入 stub（V3.1 Phase 1）
//
// 在非 Linux 平台（Windows、macOS 等）上，iptables/tc 不可用。
// 本文件提供安全的 stub 实现：记录日志但不执行实际操作。
//
// 这确保了代码在所有平台上可编译和测试，
// 同时明确告知用户真实注入仅在 Linux 上可用。
package chaos

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strings"
	"time"
)

// RealNetworkInjector 非 Linux 平台的网络故障注入器（stub 实现）
//
// 所有注入操作仅记录日志，不执行实际系统命令。
// 用于开发/测试环境以及不支持 iptables/tc 的平台。
type RealNetworkInjector struct {
	logger *slog.Logger
	iface  string
	dryRun bool
}

// RealNetworkInjectorConfig 真实注入器配置
type RealNetworkInjectorConfig struct {
	// Interface 网卡名称（非 Linux 平台忽略）
	Interface string
	// DryRun 干跑模式
	DryRun bool
	// Logger 日志器
	Logger *slog.Logger
}

// NewRealNetworkInjector 创建网络故障注入器（非 Linux stub）
func NewRealNetworkInjector(cfg RealNetworkInjectorConfig) *RealNetworkInjector {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &RealNetworkInjector{
		logger: cfg.Logger,
		iface:  cfg.Interface,
		dryRun: true, // 非 Linux 强制干跑
	}
}

// InjectDelay 注入网络延迟（非 Linux: 仅记录）
func (inj *RealNetworkInjector) InjectDelay(ctx context.Context, target string, delay, jitter time.Duration) (CleanupFunc, error) {
	if err := validateTarget(target); err != nil {
		return nil, fmt.Errorf("chaos: invalid target: %w", err)
	}

	inj.logger.Warn("网络延迟注入（非 Linux stub，仅记录）",
		"target", target,
		"delay", delay,
		"jitter", jitter,
		"platform", "non-linux",
	)

	return func(ctx context.Context) error {
		inj.logger.Info("网络延迟清理（stub）", "target", target)
		return nil
	}, nil
}

// InjectPacketLoss 注入丢包（非 Linux: 仅记录）
func (inj *RealNetworkInjector) InjectPacketLoss(ctx context.Context, target string, lossPercent int) (CleanupFunc, error) {
	if err := validateTarget(target); err != nil {
		return nil, fmt.Errorf("chaos: invalid target: %w", err)
	}
	if lossPercent < 0 || lossPercent > 100 {
		return nil, fmt.Errorf("chaos: loss percent must be 0-100, got %d", lossPercent)
	}

	inj.logger.Warn("丢包注入（非 Linux stub，仅记录）",
		"target", target,
		"loss_percent", lossPercent,
		"platform", "non-linux",
	)

	return func(ctx context.Context) error {
		inj.logger.Info("丢包清理（stub）", "target", target)
		return nil
	}, nil
}

// InjectPartition 注入网络分区（非 Linux: 仅记录）
func (inj *RealNetworkInjector) InjectPartition(ctx context.Context, target string) (CleanupFunc, error) {
	if err := validateTarget(target); err != nil {
		return nil, fmt.Errorf("chaos: invalid target: %w", err)
	}

	inj.logger.Warn("网络分区注入（非 Linux stub，仅记录）",
		"target", target,
		"platform", "non-linux",
	)

	return func(ctx context.Context) error {
		inj.logger.Info("网络分区清理（stub）", "target", target)
		return nil
	}, nil
}

// ===== 参数验证（与 Linux 版共享逻辑） =====

// hostnameRegexp 合法主机名正则
var hostnameRegexp = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.\-]*[a-zA-Z0-9]$`)

// validateTarget 验证目标地址，防止命令注入
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

	// 检查主机名
	if hostnameRegexp.MatchString(target) && len(target) <= 253 {
		return nil
	}

	return fmt.Errorf("target %q is not a valid IP, CIDR, or hostname", target)
}
