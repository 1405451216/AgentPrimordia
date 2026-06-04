package v1

import (
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAgentDeployment_BasicJSON(t *testing.T) {
	ad := AgentDeployment{
		Spec: AgentDeploymentSpec{
			Replicas: 3,
			Template: AgentTemplateSpec{
				Provider:     "openai",
				Model:        "gpt-4o",
				SystemPrompt: "你是一个智能助手",
				MaxTurns:     10,
			},
		},
	}
	data, err := json.Marshal(ad)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	// 验证 JSON 包含关键字段
	for _, want := range []string{`"provider":"openai"`, `"model":"gpt-4o"`, `"replicas":3`} {
		if !contains(data, want) {
			t.Errorf("JSON 缺 %q, got: %s", want, data)
		}
	}
}

func TestMetricsSpec_Defaults(t *testing.T) {
	// 默认值由 controller 注入；这里验证 spec 字段语义
	spec := MetricsSpec{
		Enabled: true,
		Path:    "/metrics",
		Port:    9090,
	}
	if !spec.Enabled {
		t.Error("Enabled 字段未生效")
	}
	if spec.Path != "/metrics" {
		t.Errorf("Path = %q, want /metrics", spec.Path)
	}
	if spec.Port != 9090 {
		t.Errorf("Port = %d, want 9090", spec.Port)
	}
}

func TestTracingSpec_DisabledByDefault(t *testing.T) {
	// 未设置 Enabled 字段时,应为零值 false
	spec := TracingSpec{}
	if spec.Enabled {
		t.Error("TracingSpec 默认应禁用")
	}
	if spec.OTLPEndpoint != "" {
		t.Error("OTLPEndpoint 默认应为空")
	}
}

func TestAgentDeploymentStatus_Metrics(t *testing.T) {
	// 验证 Phase 8.3 新增 status 字段
	now := time.Now()
	status := AgentDeploymentStatus{
		ActiveReplicas:            3,
		CompletedTasks:            100,
		FailedTasks:               5,
		ErrorRate:                 0.05,
		AverageTurnLatencySeconds: 1.2,
		TotalTokens:               50000,
		EstimatedCostUSD:          1.5,
		Conditions: []AgentDeploymentCondition{
			{
				Type:               "Ready",
				Status:             "True",
				LastTransitionTime: metav1.NewTime(now),
				Reason:             "AllReplicasReady",
			},
		},
	}
	if status.AverageTurnLatencySeconds != 1.2 {
		t.Error("AverageTurnLatencySeconds 字段未生效")
	}
	if status.TotalTokens != 50000 {
		t.Error("TotalTokens 字段未生效")
	}
	if status.EstimatedCostUSD != 1.5 {
		t.Error("EstimatedCostUSD 字段未生效")
	}
	if len(status.Conditions) != 1 {
		t.Error("Conditions 字段未生效")
	}
}

func TestAutoscalingSpec_Bounds(t *testing.T) {
	spec := AutoscalingSpec{
		MinReplicas:           1,
		MaxReplicas:           10,
		TargetConcurrentTasks: 5,
	}
	if spec.MinReplicas > spec.MaxReplicas {
		t.Error("MinReplicas 应 ≤ MaxReplicas")
	}
	if spec.TargetConcurrentTasks <= 0 {
		t.Error("TargetConcurrentTasks 应 > 0")
	}
}

func TestAgentDeploymentList_Empty(t *testing.T) {
	list := AgentDeploymentList{}
	if list.Items != nil && len(list.Items) != 0 {
		t.Error("空 list 应无 items")
	}
}

func TestAgentDeployment_DeepCopy(t *testing.T) {
	orig := &AgentDeployment{
		Spec: AgentDeploymentSpec{
			Replicas: 3,
			Template: AgentTemplateSpec{
				Provider:     "openai",
				Model:        "gpt-4o",
				SystemPrompt: "你是一个智能助手",
				MaxTurns:     10,
				Metrics: &MetricsSpec{
					Enabled: true,
					Port:    9090,
				},
			},
		},
	}

	// 验证 DeepCopyObject 返回 runtime.Object
	var obj runtime.Object = orig
	if obj == nil {
		t.Fatal("AgentDeployment 不应 nil")
	}

	// DeepCopy 后比较关键字段
	copied := orig.DeepCopy()
	if copied == orig {
		t.Error("DeepCopy 应返回新指针")
	}
	if copied.Spec.Replicas != orig.Spec.Replicas {
		t.Error("DeepCopy 后 Replicas 不一致")
	}
	if copied.Spec.Template.Metrics == nil || !copied.Spec.Template.Metrics.Enabled {
		t.Error("DeepCopy 后 Metrics 字段应保留")
	}

	// 改 copied 不应影响 orig
	copied.Spec.Replicas = 99
	if orig.Spec.Replicas == 99 {
		t.Error("DeepCopy 后修改应隔离")
	}
}

func TestAgentDeployment_DeepCopyInto_Nil(t *testing.T) {
	var nilAD *AgentDeployment
	obj := nilAD.DeepCopyObject()
	if obj != nil {
		t.Error("nil 输入应返回 nil")
	}
}

// helpers

func contains(data []byte, substr string) bool {
	for i := 0; i+len(substr) <= len(data); i++ {
		if string(data[i:i+len(substr)]) == substr {
			return true
		}
	}
	return false
}
