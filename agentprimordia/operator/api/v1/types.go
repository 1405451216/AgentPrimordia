// Package v1 定义 AgentDeployment CRD 的类型
package v1

// 本文件包含 AgentDeployment 相关类型定义。
// DeepCopy 实现见 zz_generated_deepcopy.go (Phase 8.3 手写以
// 替代 controller-gen 自动生成)。

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentDeployment 是 Agent 部署的 CRD
//
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
type AgentDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentDeploymentSpec   `json:"spec,omitempty"`
	Status AgentDeploymentStatus `json:"status,omitempty"`
}

// AgentDeploymentSpec 定义 Agent 部署的期望状态
type AgentDeploymentSpec struct {
	// 副本数量
	Replicas int32 `json:"replicas"`

	// Agent 模板
	Template AgentTemplateSpec `json:"template"`

	// 自动扩缩容配置
	// +optional
	Autoscaling *AutoscalingSpec `json:"autoscaling,omitempty"`

	// 健康检查配置
	// +optional
	HealthCheck *HealthCheckSpec `json:"healthCheck,omitempty"`
}

// AgentTemplateSpec 定义 Agent 的配置模板
type AgentTemplateSpec struct {
	// LLM 提供者名称 (openai, anthropic, gemini, ollama, azure)
	Provider string `json:"provider"`

	// LLM 模型名称
	Model string `json:"model"`

	// 系统提示词
	SystemPrompt string `json:"systemPrompt"`

	// 最大推理轮次
	// +optional
	MaxTurns int32 `json:"maxTurns,omitempty"`

	// API Key 引用的 Secret 名称
	// +optional
	APISecretRef string `json:"apiSecretRef,omitempty"`

	// 工具配置列表
	// +optional
	Tools []ToolSpec `json:"tools,omitempty"`

	// 记忆存储配置
	// +optional
	Memory *MemorySpec `json:"memory,omitempty"`

	// 资源限制
	// +optional
	Resources ResourceSpec `json:"resources,omitempty"`

	// 指标暴露配置 (Phase 8.3 新增)
	// +optional
	Metrics *MetricsSpec `json:"metrics,omitempty"`

	// 追踪配置 (Phase 8.3 新增)
	// +optional
	Tracing *TracingSpec `json:"tracing,omitempty"`
}

// MetricsSpec 定义 Prometheus 指标暴露配置
type MetricsSpec struct {
	// 是否启用指标端点
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// 指标 HTTP 端点路径,默认 /metrics
	// +optional
	Path string `json:"path,omitempty"`

	// 指标 HTTP 端点端口,默认 9090
	// +optional
	Port int32 `json:"port,omitempty"`

	// ServiceMonitor 配置(对接 Prometheus Operator)
	// +optional
	ServiceMonitor *ServiceMonitorSpec `json:"serviceMonitor,omitempty"`
}

// ServiceMonitorSpec 定义 Prometheus Operator 的 ServiceMonitor 引用
type ServiceMonitorSpec struct {
	// ServiceMonitor 名称
	Name string `json:"name,omitempty"`

	// 抓取间隔 (如 "30s")
	// +optional
	Interval string `json:"interval,omitempty"`
}

// TracingSpec 定义 OpenTelemetry 追踪配置
type TracingSpec struct {
	// 是否启用追踪
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// OTLP 端点(如 http://otel-collector:4317)
	// +optional
	OTLPEndpoint string `json:"otlpEndpoint,omitempty"`

	// 采样率 (0-1)
	// +optional
	SamplingRate float64 `json:"samplingRate,omitempty"`
}

// ToolSpec 定义单个工具的配置
type ToolSpec struct {
	// 工具名称 (filesystem, shell, web, knowledge)
	Name string `json:"name"`

	// 工具配置参数
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// MemorySpec 定义记忆存储配置
type MemorySpec struct {
	// 后端类型 (sqlite, memory)
	Backend string `json:"backend"`

	// 存储大小限制
	// +optional
	SizeLimit string `json:"sizeLimit,omitempty"`

	// 持久化卷声明名称
	// +optional
	PVCName string `json:"pvcName,omitempty"`
}

// ResourceSpec 定义资源请求和限制
type ResourceSpec struct {
	Requests ResourceQuantities `json:"requests,omitempty"`
	Limits   ResourceQuantities `json:"limits,omitempty"`
}

// ResourceQuantities 定义 CPU 和内存数量
type ResourceQuantities struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// AutoscalingSpec 定义自动扩缩容配置
type AutoscalingSpec struct {
	// 最小副本数
	MinReplicas int32 `json:"minReplicas"`

	// 最大副本数
	MaxReplicas int32 `json:"maxReplicas"`

	// 每个副本的目标并发任务数
	TargetConcurrentTasks int32 `json:"targetConcurrentTasks"`
}

// HealthCheckSpec 定义健康检查配置
type HealthCheckSpec struct {
	// 存活探针
	// +optional
	LivenessProbe *ProbeSpec `json:"livenessProbe,omitempty"`

	// 就绪探针
	// +optional
	ReadinessProbe *ProbeSpec `json:"readinessProbe,omitempty"`
}

// ProbeSpec 定义 HTTP 探针配置
type ProbeSpec struct {
	HTTPGet             *HTTPGetAction `json:"httpGet,omitempty"`
	InitialDelaySeconds int32          `json:"initialDelaySeconds,omitempty"`
	TimeoutSeconds      int32          `json:"timeoutSeconds,omitempty"`
	PeriodSeconds       int32          `json:"periodSeconds,omitempty"`
	SuccessThreshold    int32          `json:"successThreshold,omitempty"`
	FailureThreshold    int32          `json:"failureThreshold,omitempty"`
}

// HTTPGetAction 定义 HTTP GET 探针
type HTTPGetAction struct {
	Path string `json:"path"`
	Port int32  `json:"port"`
}

// AgentDeploymentStatus 定义 Agent 部署的观测状态
type AgentDeploymentStatus struct {
	// 活跃副本数
	ActiveReplicas int32 `json:"activeReplicas"`

	// 已完成任务总数
	CompletedTasks int64 `json:"completedTasks"`

	// 失败任务总数
	FailedTasks int64 `json:"failedTasks"`

	// 错误率 (0-1)
	ErrorRate float64 `json:"errorRate"`

	// 平均轮次延迟 (秒)
	// +optional (Phase 8.3 新增)
	AverageTurnLatencySeconds float64 `json:"averageTurnLatencySeconds,omitempty"`

	// 累计 LLM Token 消耗 (prompt + completion)
	// +optional (Phase 8.3 新增)
	TotalTokens int64 `json:"totalTokens,omitempty"`

	// 累计估算成本 (USD)
	// +optional (Phase 8.3 新增)
	EstimatedCostUSD float64 `json:"estimatedCostUSD,omitempty"`

	// 条件列表
	Conditions []AgentDeploymentCondition `json:"conditions,omitempty"`
}

// AgentDeploymentCondition 描述部署的一个条件状态
type AgentDeploymentCondition struct {
	// 条件类型
	Type string `json:"type"`

	// 条件状态 (True, False, Unknown)
	Status string `json:"status"`

	// 上次转换时间
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// 原因
	Reason string `json:"reason,omitempty"`

	// 消息
	Message string `json:"message,omitempty"`
}

// AgentDeploymentList 是 AgentDeployment 的列表
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AgentDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AgentDeployment `json:"items"`
}
