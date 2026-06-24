// Stability: Stable — 断路器模式。
package ap

import "agentprimordia/internal/resilience"

// CircuitBreaker 断路器，用于 LLM Provider 故障转移
type CircuitBreaker = resilience.CircuitBreaker

// CircuitBreakerConfig 断路器配置
type CircuitBreakerConfig = resilience.Config

// CircuitBreakerState 断路器状态
type CircuitBreakerState = resilience.State

// NewCircuitBreaker 创建断路器
var NewCircuitBreaker = resilience.NewCircuitBreaker

const (
	// CircuitClosed 断路器关闭（正常）
	CircuitClosed = resilience.StateClosed
	// CircuitOpen 断路器打开（断路）
	CircuitOpen = resilience.StateOpen
	// CircuitHalfOpen 断路器半开（试探）
	CircuitHalfOpen = resilience.StateHalfOpen
)
