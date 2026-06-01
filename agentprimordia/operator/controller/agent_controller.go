// Package controller 实现 AgentDeployment 的调谐逻辑
package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1 "agentprimordia/operator/api/v1"
)

// AgentDeploymentReconciler 调谐 AgentDeployment 资源
type AgentDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.primordia.dev,resources=agentdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.primordia.dev,resources=agentdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile 调谐 AgentDeployment 到期望状态
func (r *AgentDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 获取 AgentDeployment
	var agentDeploy agentv1.AgentDeployment
	if err := r.Get(ctx, req.NamespacedName, &agentDeploy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("调谐 AgentDeployment", "name", agentDeploy.Name, "replicas", agentDeploy.Spec.Replicas)

	// 1. 确保 ConfigMap 存在（包含 .ap.yaml 配置）
	if err := r.ensureConfigMap(ctx, &agentDeploy); err != nil {
		return ctrl.Result{}, fmt.Errorf("创建 ConfigMap 失败: %w", err)
	}

	// 2. 确保 Deployment 存在
	if err := r.ensureDeployment(ctx, &agentDeploy); err != nil {
		return ctrl.Result{}, fmt.Errorf("创建 Deployment 失败: %w", err)
	}

	// 3. 更新状态
	if err := r.updateStatus(ctx, &agentDeploy); err != nil {
		return ctrl.Result{}, fmt.Errorf("更新状态失败: %w", err)
	}

	return ctrl.Result{}, nil
}

// ensureConfigMap 创建或更新 Agent 配置的 ConfigMap
func (r *AgentDeploymentReconciler) ensureConfigMap(ctx context.Context, agentDeploy *agentv1.AgentDeployment) error {
	cmName := fmt.Sprintf("%s-config", agentDeploy.Name)
	var existingCM corev1.ConfigMap

	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentDeploy.Namespace}, &existingCM)
	if err == nil {
		return nil // 已存在
	}

	// 构建 .ap.yaml 内容
	apConfig := fmt.Sprintf(`name: %s
llm:
  provider: %s
  model: %s
agent:
  max_turns: %d
  system_prompt: %q
`,
		agentDeploy.Name,
		agentDeploy.Spec.Template.Provider,
		agentDeploy.Spec.Template.Model,
		agentDeploy.Spec.Template.MaxTurns,
		agentDeploy.Spec.Template.SystemPrompt,
	)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: agentDeploy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(agentDeploy, agentv1.SchemeGroupVersion.WithKind("AgentDeployment")),
			},
		},
		Data: map[string]string{
			"ap.yaml": apConfig,
		},
	}

	return r.Create(ctx, cm)
}

// ensureDeployment 创建或更新 Agent 的 Deployment
func (r *AgentDeploymentReconciler) ensureDeployment(ctx context.Context, agentDeploy *agentv1.AgentDeployment) error {
	deployName := fmt.Sprintf("%s-agent", agentDeploy.Name)
	var existingDeploy appsv1.Deployment

	err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: agentDeploy.Namespace}, &existingDeploy)
	if err == nil {
		// 已存在，检查是否需要更新副本数
		if *existingDeploy.Spec.Replicas != agentDeploy.Spec.Replicas {
			existingDeploy.Spec.Replicas = &agentDeploy.Spec.Replicas
			return r.Update(ctx, &existingDeploy)
		}
		return nil
	}

	// 构建环境变量
	envVars := []corev1.EnvVar{
		{
			Name: "AP_CONFIG_PATH",
			Value: "/etc/ap/ap.yaml",
		},
	}

	// 从 Secret 引用 API Key
	if agentDeploy.Spec.Template.APISecretRef != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "OPENAI_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: agentDeploy.Spec.Template.APISecretRef,
					},
					Key: "api-key",
				},
			},
		})
	}

	// 资源配置
	resources := corev1.ResourceRequirements{}
	if agentDeploy.Spec.Template.Resources.Requests.CPU != "" || agentDeploy.Spec.Template.Resources.Requests.Memory != "" {
		// 由 K8s 自动解析资源量
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: agentDeploy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(agentDeploy, agentv1.SchemeGroupVersion.WithKind("AgentDeployment")),
			},
			Labels: map[string]string{
				"app":           "agentprimordia",
				"agent-deploy":  agentDeploy.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &agentDeploy.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":          "agentprimordia",
					"agent-deploy": agentDeploy.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":          "agentprimordia",
						"agent-deploy": agentDeploy.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "agent",
							Image: "ghcr.io/agentprimordia/agentprimordia:latest",
							Command: []string{"./ap", "run"},
							Env:   envVars,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/ap",
									ReadOnly:  true,
								},
							},
							Resources: resources,
						},
						// Metrics sidecar
						{
							Name:  "metrics",
							Image: "ghcr.io/agentprimordia/agentprimordia:latest",
							Command: []string{"./ap", "debug", "--port", "9090"},
							Ports: []corev1.ContainerPort{
								{ContainerPort: 9090, Name: "metrics"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", agentDeploy.Name),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return r.Create(ctx, deploy)
}

// updateStatus 更新 AgentDeployment 的状态
func (r *AgentDeploymentReconciler) updateStatus(ctx context.Context, agentDeploy *agentv1.AgentDeployment) error {
	deployName := fmt.Sprintf("%s-agent", agentDeploy.Name)
	var deploy appsv1.Deployment

	if err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: agentDeploy.Namespace}, &deploy); err != nil {
		return err
	}

	agentDeploy.Status.ActiveReplicas = deploy.Status.ReadyReplicas

	condition := agentv1.AgentDeploymentCondition{
		Type:               "Available",
		Status:             "True",
		LastTransitionTime: metav1.Now(),
		Reason:             "MinimumReplicasAvailable",
		Message:            fmt.Sprintf("Deployment has %d ready replicas", deploy.Status.ReadyReplicas),
	}

	if deploy.Status.ReadyReplicas < *deploy.Spec.Replicas {
		condition.Status = "False"
		condition.Reason = "MinimumReplicasUnavailable"
	}

	agentDeploy.Status.Conditions = []agentv1.AgentDeploymentCondition{condition}

	return r.Status().Update(ctx, agentDeploy)
}

// SetupWithManager 注册 Controller 到 Manager
func (r *AgentDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1.AgentDeployment{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
