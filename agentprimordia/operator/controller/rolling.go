// Package controller - 滚动升级 / 优雅关闭
//
// 文件：rolling.go
// 作用：在 ensureDeployment 中组装 Deployment.Spec.Strategy 与 Pod 级别的
//
//	TerminationGracePeriodSeconds / Lifecycle.PreStop，使滚动升级过程中
//	新 Pod 先启动并通过健康检查后才终止旧 Pod，避免服务中断。
//
// 设计：
//   - 默认 Strategy：RollingUpdate，MaxUnavailable=1, MaxSurge=1
//   - 默认 TerminationGracePeriodSeconds：30s
//   - 默认 preStop hook：sleep 5s（等待 Service Endpoints 同步删除本 Pod IP）
//
// 为什么需要 preStop：K8s 在 Pod 终止时同时做两件事：(1) 从 Service Endpoints
// 移除 Pod IP；(2) 发送 SIGTERM。这两件事是并行的，导致仍可能有流量被路由到
// 已 SIGTERM 的 Pod。preStop sleep 让步骤 (1) 先完成，然后再杀进程。
package controller

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// defaultMaxUnavailable / defaultMaxSurge 滚动升级并发度
// 设为绝对值 1 以避免小规模部署出现"所有 Pod 同时被替换"的极端情况
const (
	defaultMaxUnavailable = 1
	defaultMaxSurge       = 1
)

// defaultTerminationGracePeriodSeconds Pod 优雅关闭宽限期（秒）
// 30s 足以让 in-flight 请求完成 + preStop hook 执行
const defaultTerminationGracePeriodSeconds int64 = 30

// defaultPreStopSleepSeconds preStop hook sleep 时长（秒）
// 5s 足够 Service endpoints controller 同步删除本 Pod IP（典型 1-3s）
const defaultPreStopSleepSeconds int32 = 5

// buildDeploymentStrategy 构造默认 RollingUpdate 策略
func buildDeploymentStrategy() appsv1.DeploymentStrategy {
	mu := intstr.FromInt(defaultMaxUnavailable)
	ms := intstr.FromInt(defaultMaxSurge)
	return appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &mu,
			MaxSurge:       &ms,
		},
	}
}

// applyTerminationLifecycle 为 PodSpec 注入 terminationGracePeriodSeconds + preStop
//
// 该函数原地修改 podSpec（按惯例 Deployment 内的 Pod 是值传递，因此原值会被覆盖）。
func applyTerminationLifecycle(podSpec *corev1.PodSpec) {
	if podSpec == nil {
		return
	}
	if podSpec.TerminationGracePeriodSeconds == nil {
		grace := defaultTerminationGracePeriodSeconds
		podSpec.TerminationGracePeriodSeconds = &grace
	}
	// 为每个容器加 preStop hook（已有则跳过）
	for i := range podSpec.Containers {
		c := &podSpec.Containers[i]
		if c.Lifecycle != nil && c.Lifecycle.PreStop != nil {
			continue
		}
		if c.Lifecycle == nil {
			c.Lifecycle = &corev1.Lifecycle{}
		}
		c.Lifecycle.PreStop = &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/sh", "-c", "sleep 5"},
			},
		}
	}
}

// hasRollingUpdate 检查 Deployment 是否已配置 RollingUpdate 策略
func hasRollingUpdate(d *appsv1.Deployment) bool {
	return d.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType &&
		d.Spec.Strategy.RollingUpdate != nil
}

// hasPreStopHook 检查 Container 是否已配置 preStop
func hasPreStopHook(c corev1.Container) bool {
	return c.Lifecycle != nil && c.Lifecycle.PreStop != nil
}

// rollingStrategyEqual 判断两个 strategy 是否等价
func rollingStrategyEqual(a, b appsv1.DeploymentStrategy) bool {
	if a.Type != b.Type {
		return false
	}
	if a.RollingUpdate == nil && b.RollingUpdate == nil {
		return true
	}
	if a.RollingUpdate == nil || b.RollingUpdate == nil {
		return false
	}
	return intstrEqualPtr(a.RollingUpdate.MaxUnavailable, b.RollingUpdate.MaxUnavailable) &&
		intstrEqualPtr(a.RollingUpdate.MaxSurge, b.RollingUpdate.MaxSurge)
}

// allContainersHavePreStop 检查 PodSpec 所有容器是否都已配置 preStop hook
//
// 该函数用于检测是否需要 applyTerminationLifecycle 注入新的 preStop。
func allContainersHavePreStop(spec corev1.PodSpec) bool {
	for _, c := range spec.Containers {
		if !hasPreStopHook(c) {
			return false
		}
	}
	return true
}

// int64PtrEqual 判断两个 *int64 是否等价（用于 TerminationGracePeriodSeconds 比较）
func int64PtrEqual(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
