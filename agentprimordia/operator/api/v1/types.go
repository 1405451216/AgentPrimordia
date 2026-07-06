// Package v1 定义 AgentDeployment CRD 的类型
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// AgentDeployment 是 Agent 部署的 CRD
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +genclient
type AgentDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentDeploymentSpec   `json:"spec,omitempty"`
	Status AgentDeploymentStatus `json:"status,omitempty"`
}

// AgentDeploymentSpec 定义 Agent 部署的期望状态
// +kubebuilder:object:generate=true
type AgentDeploymentSpec struct {
	// 副本数量
	Replicas int32 `json:"replicas"`

	// Agent 模板
	Template AgentTemplateSpec `json:"template"`

	// 自动扩缩容配置（HPA，基于 concurrent_tasks_per_pod 自定义指标）
	// +optional
	Autoscaling *AutoscalingSpec `json:"autoscaling,omitempty"`

	// 健康检查配置
	// +optional
	HealthCheck *HealthCheckSpec `json:"healthCheck,omitempty"`

	// PodDisruptionBudget 配置（Phase 4 Task 7 新增）
	// +optional
	DisruptionBudget *DisruptionBudgetSpec `json:"disruptionBudget,omitempty"`
}

// DisruptionBudgetSpec 定义 Pod Disruption Budget 配置
//
// MinAvailable / MaxUnavailable 二选一。Replica 为 1 时 controller 会自动跳过 PDB 创建，
// 因为 PDB 至少需要 2 个 Pod 才能生效。
// +kubebuilder:object:generate=true
type DisruptionBudgetSpec struct {
	// MinAvailable 最小可用 Pod 数（绝对值或百分比字符串如 "50%"）
	// +optional
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`

	// MaxUnavailable 最大不可用 Pod 数（绝对值或百分比字符串）
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// Enabled 显式启用/禁用 PDB，nil 时按默认规则（replicas >= 2 自动启用）
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// AgentTemplateSpec 定义 Agent 的配置模板
// +kubebuilder:object:generate=true
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

	// 容器镜像覆盖，不设置时使用 controller 默认值
	// +optional
	Image string `json:"image,omitempty"`

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
// +kubebuilder:object:generate=true
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
// +kubebuilder:object:generate=true
type ServiceMonitorSpec struct {
	// ServiceMonitor 名称
	Name string `json:"name,omitempty"`

	// 抓取间隔 (如 "30s")
	// +optional
	Interval string `json:"interval,omitempty"`
}

// TracingSpec 定义 OpenTelemetry 追踪配置
// +kubebuilder:object:generate=true
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
// +kubebuilder:object:generate=true
type ToolSpec struct {
	// 工具名称 (filesystem, shell, web, knowledge)
	Name string `json:"name"`

	// 工具配置参数
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// MemorySpec 定义记忆存储配置
// +kubebuilder:object:generate=true
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
// +kubebuilder:object:generate=true
type ResourceSpec struct {
	Requests ResourceQuantities `json:"requests,omitempty"`
	Limits   ResourceQuantities `json:"limits,omitempty"`
}

// ResourceQuantities 定义 CPU 和内存数量
// +kubebuilder:object:generate=true
type ResourceQuantities struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// AutoscalingSpec 定义自动扩缩容配置
// +kubebuilder:object:generate=true
type AutoscalingSpec struct {
	// 最小副本数
	MinReplicas int32 `json:"minReplicas"`

	// 最大副本数
	MaxReplicas int32 `json:"maxReplicas"`

	// 每个副本的目标并发任务数
	TargetConcurrentTasks int32 `json:"targetConcurrentTasks"`

	// HPA 行为配置（Phase 4 Task 8 新增）
	// +optional
	Behavior *HPABehaviorSpec `json:"behavior,omitempty"`
}

// HPABehaviorSpec 定义 HPA Behavior 字段
//
// 稳定窗口（StabilizationWindowSeconds）控制缩容/扩容决策的时间窗口，
// 避免短时间内反复抖动。Policy 控制单次扩缩容的步进幅度与频率。
//
// 不配置时使用 controller 默认（缩容 5min/25%、扩容 30s/100%）。
// +kubebuilder:object:generate=true
type HPABehaviorSpec struct {
	// ScaleDown 缩容策略
	// +optional
	ScaleDown *HPAScalingRulesSpec `json:"scaleDown,omitempty"`

	// ScaleUp 扩容策略
	// +optional
	ScaleUp *HPAScalingRulesSpec `json:"scaleUp,omitempty"`
}

// HPAScalingRulesSpec 定义单方向（缩容/扩容）的扩缩规则
// +kubebuilder:object:generate=true
type HPAScalingRulesSpec struct {
	// StabilizationWindowSeconds 稳定窗口（秒）。在该窗口内的扩缩决策会被平滑。
	// +optional
	StabilizationWindowSeconds *int32 `json:"stabilizationWindowSeconds,omitempty"`

	// Policies 扩缩策略列表，按顺序尝试直到找到可执行的
	// +optional
	Policies []HPAScalingPolicySpec `json:"policies,omitempty"`

	// SelectPolicy 选择策略：Max / Min / Disabled
	// +optional
	SelectPolicy *string `json:"selectPolicy,omitempty"`
}

// HPAScalingPolicySpec 定义单条扩缩策略
// +kubebuilder:object:generate=true
type HPAScalingPolicySpec struct {
	// Type 策略类型：Pods / Percent
	// +optional
	Type string `json:"type,omitempty"`

	// Value 数值（Pods 时为绝对数，Percent 时为百分比 0-100）
	Value int32 `json:"value"`

	// PeriodSeconds 应用周期（秒）
	// +optional
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
}

// HealthCheckSpec 定义健康检查配置
// +kubebuilder:object:generate=true
type HealthCheckSpec struct {
	// 存活探针
	// +optional
	LivenessProbe *ProbeSpec `json:"livenessProbe,omitempty"`

	// 就绪探针
	// +optional
	ReadinessProbe *ProbeSpec `json:"readinessProbe,omitempty"`
}

// ProbeSpec 定义 HTTP 探针配置
// +kubebuilder:object:generate=true
type ProbeSpec struct {
	HTTPGet             *HTTPGetAction `json:"httpGet,omitempty"`
	InitialDelaySeconds int32          `json:"initialDelaySeconds,omitempty"`
	TimeoutSeconds      int32          `json:"timeoutSeconds,omitempty"`
	PeriodSeconds       int32          `json:"periodSeconds,omitempty"`
	SuccessThreshold    int32          `json:"successThreshold,omitempty"`
	FailureThreshold    int32          `json:"failureThreshold,omitempty"`
}

// HTTPGetAction 定义 HTTP GET 探针
// +kubebuilder:object:generate=true
type HTTPGetAction struct {
	Path string `json:"path"`
	Port int32  `json:"port"`
}

// AgentDeploymentStatus 定义 Agent 部署的观测状态
// +kubebuilder:object:generate=true
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
// +kubebuilder:object:generate=true
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
// +kubebuilder:object:root=true
type AgentDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []AgentDeployment `json:"items"`
}
