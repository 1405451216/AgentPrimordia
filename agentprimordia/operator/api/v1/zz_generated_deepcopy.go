// Package v1 类型 DeepCopy 实现
// 正常情况下由 controller-gen 自动生成 (Phase 8.3 手写以让 operator 编译)。
//
// 后续可改回 codegen: `controller-gen object paths=./api/v1/...`
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ===== AgentDeployment =====

// DeepCopyObject 实现 runtime.Object 接口
func (in *AgentDeployment) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AgentDeployment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopy 深拷贝 AgentDeployment
func (in *AgentDeployment) DeepCopy() *AgentDeployment {
	if in == nil {
		return nil
	}
	out := new(AgentDeployment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto 拷贝到 out
func (in *AgentDeployment) DeepCopyInto(out *AgentDeployment) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// ===== AgentDeploymentList =====

func (in *AgentDeploymentList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(AgentDeploymentList)
	in.DeepCopyInto(out)
	return out
}

func (in *AgentDeploymentList) DeepCopy() *AgentDeploymentList {
	if in == nil {
		return nil
	}
	out := new(AgentDeploymentList)
	in.DeepCopyInto(out)
	return out
}

func (in *AgentDeploymentList) DeepCopyInto(out *AgentDeploymentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AgentDeployment, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// ===== AgentDeploymentSpec / Status / Template =====

func (in *AgentDeploymentSpec) DeepCopyInto(out *AgentDeploymentSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
	if in.Autoscaling != nil {
		out.Autoscaling = in.Autoscaling.DeepCopy()
	}
	if in.HealthCheck != nil {
		out.HealthCheck = in.HealthCheck.DeepCopy()
	}
}

func (in *AgentTemplateSpec) DeepCopyInto(out *AgentTemplateSpec) {
	*out = *in
	if in.Tools != nil {
		out.Tools = make([]ToolSpec, len(in.Tools))
		copy(out.Tools, in.Tools)
	}
	if in.Memory != nil {
		out.Memory = in.Memory.DeepCopy()
	}
	if in.Metrics != nil {
		out.Metrics = in.Metrics.DeepCopy()
	}
	if in.Tracing != nil {
		out.Tracing = in.Tracing.DeepCopy()
	}
}

func (in *AgentDeploymentStatus) DeepCopyInto(out *AgentDeploymentStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]AgentDeploymentCondition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// ===== 指针类型的 DeepCopy =====

func (in *ToolSpec) DeepCopy() *ToolSpec {
	if in == nil {
		return nil
	}
	out := *in
	if in.Config != nil {
		out.Config = make(map[string]string, len(in.Config))
		for k, v := range in.Config {
			out.Config[k] = v
		}
	}
	return &out
}

func (in *MemorySpec) DeepCopy() *MemorySpec {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (in *MetricsSpec) DeepCopy() *MetricsSpec {
	if in == nil {
		return nil
	}
	out := *in
	if in.ServiceMonitor != nil {
		out.ServiceMonitor = in.ServiceMonitor.DeepCopy()
	}
	return &out
}

func (in *ServiceMonitorSpec) DeepCopy() *ServiceMonitorSpec {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (in *TracingSpec) DeepCopy() *TracingSpec {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (in *AutoscalingSpec) DeepCopy() *AutoscalingSpec {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (in *HealthCheckSpec) DeepCopy() *HealthCheckSpec {
	if in == nil {
		return nil
	}
	out := *in
	if in.LivenessProbe != nil {
		out.LivenessProbe = in.LivenessProbe.DeepCopy()
	}
	if in.ReadinessProbe != nil {
		out.ReadinessProbe = in.ReadinessProbe.DeepCopy()
	}
	return &out
}

func (in *ProbeSpec) DeepCopy() *ProbeSpec {
	if in == nil {
		return nil
	}
	out := *in
	if in.HTTPGet != nil {
		out.HTTPGet = in.HTTPGet.DeepCopy()
	}
	return &out
}

func (in *HTTPGetAction) DeepCopy() *HTTPGetAction {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (in *ResourceSpec) DeepCopyInto(out *ResourceSpec) {
	*out = *in
}

func (in *ResourceQuantities) DeepCopyInto(out *ResourceQuantities) {
	*out = *in
}

func (in *AgentDeploymentCondition) DeepCopyInto(out *AgentDeploymentCondition) {
	*out = *in
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// 保留 metav1 引用(用于 LastTransitionTime)
var _ = metav1.Time{}
