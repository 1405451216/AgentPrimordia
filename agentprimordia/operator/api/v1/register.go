package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// GroupName 是 API 组名
	GroupName = "agent.primordia.dev"
	// Version 是 API 版本
	Version = "v1"
)

// SchemeGroupVersion 是组的版本
var SchemeGroupVersion = schema.GroupVersion{
	Group:   GroupName,
	Version: Version,
}

// Resource 获取 GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder 注册类型到 Scheme
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme 将所有类型添加到 Scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&AgentDeployment{},
		&AgentDeploymentList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
