// Package controller 实现 AgentDeployment 的调谐逻辑
package controller

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1 "agentprimordia/operator/api/v1"
)

const agentFinalizer = "agent.primordia.dev/finalizer"

// AgentDeploymentReconciler 调谐 AgentDeployment 资源
type AgentDeploymentReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	DefaultImage string
}

// +kubebuilder:rbac:groups=agent.primordia.dev,resources=agentdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.primordia.dev,resources=agentdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete

// Reconcile 调谐 AgentDeployment 到期望状态
func (r *AgentDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 获取 AgentDeployment
	var agentDeploy agentv1.AgentDeployment
	if err := r.Get(ctx, req.NamespacedName, &agentDeploy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 处理删除（Finalizer 逻辑）
	if !agentDeploy.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&agentDeploy, agentFinalizer) {
			logger.Info("清理 AgentDeployment 资源", "name", agentDeploy.Name)
			controllerutil.RemoveFinalizer(&agentDeploy, agentFinalizer)
			if err := r.Update(ctx, &agentDeploy); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 添加 Finalizer
	if !controllerutil.ContainsFinalizer(&agentDeploy, agentFinalizer) {
		controllerutil.AddFinalizer(&agentDeploy, agentFinalizer)
		if err := r.Update(ctx, &agentDeploy); err != nil {
			return ctrl.Result{}, err
		}
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

	// 3. 确保 Service 存在（暴露 metrics sidecar）
	if err := r.ensureService(ctx, &agentDeploy); err != nil {
		return ctrl.Result{}, fmt.Errorf("创建 Service 失败: %w", err)
	}

	// 4. 确保 HPA 存在（如果配置了自动扩缩容）
	if err := r.ensureHPA(ctx, &agentDeploy); err != nil {
		return ctrl.Result{}, fmt.Errorf("创建 HPA 失败: %w", err)
	}

	// 5. 更新状态
	if err := r.updateStatus(ctx, &agentDeploy); err != nil {
		return ctrl.Result{}, fmt.Errorf("更新状态失败: %w", err)
	}

	return ctrl.Result{}, nil
}

// imageOrDefault 返回容器镜像，优先使用 spec 中的配置
func (r *AgentDeploymentReconciler) imageOrDefault(ad *agentv1.AgentDeployment) string {
	if ad.Spec.Template.Image != "" {
		return ad.Spec.Template.Image
	}
	if r.DefaultImage != "" {
		return r.DefaultImage
	}
	return "ghcr.io/agentprimordia/agentprimordia:latest"
}

// ensureConfigMap 创建或更新 Agent 配置的 ConfigMap
func (r *AgentDeploymentReconciler) ensureConfigMap(ctx context.Context, agentDeploy *agentv1.AgentDeployment) error {
	cmName := fmt.Sprintf("%s-config", agentDeploy.Name)
	var existingCM corev1.ConfigMap

	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentDeploy.Namespace}, &existingCM)
	if err == nil {
		return nil // 已存在
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("检查 ConfigMap 失败: %w", err)
	}

	// 使用 yaml.Marshal 安全生成 YAML 配置
	apConfigData := map[string]any{
		"name": agentDeploy.Name,
		"llm": map[string]any{
			"provider": agentDeploy.Spec.Template.Provider,
			"model":    agentDeploy.Spec.Template.Model,
		},
		"agent": map[string]any{
			"max_turns":     agentDeploy.Spec.Template.MaxTurns,
			"system_prompt": agentDeploy.Spec.Template.SystemPrompt,
		},
	}
	apConfigBytes, err := yaml.Marshal(apConfigData)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: agentDeploy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(agentDeploy, agentv1.SchemeGroupVersion.WithKind("AgentDeployment")),
			},
		},
		Data: map[string]string{
			"ap.yaml": string(apConfigBytes),
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
		if existingDeploy.Spec.Replicas == nil || *existingDeploy.Spec.Replicas != agentDeploy.Spec.Replicas {
			existingDeploy.Spec.Replicas = &agentDeploy.Spec.Replicas
			return r.Update(ctx, &existingDeploy)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("检查 Deployment 失败: %w", err)
	}

	// 构建环境变量
	envVars := []corev1.EnvVar{
		{
			Name:  "AP_CONFIG_PATH",
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
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
	if agentDeploy.Spec.Template.Resources.Requests.CPU != "" {
		resources.Requests[corev1.ResourceCPU] = resource.MustParse(agentDeploy.Spec.Template.Resources.Requests.CPU)
	}
	if agentDeploy.Spec.Template.Resources.Requests.Memory != "" {
		resources.Requests[corev1.ResourceMemory] = resource.MustParse(agentDeploy.Spec.Template.Resources.Requests.Memory)
	}
	if agentDeploy.Spec.Template.Resources.Limits.CPU != "" {
		resources.Limits[corev1.ResourceCPU] = resource.MustParse(agentDeploy.Spec.Template.Resources.Limits.CPU)
	}
	if agentDeploy.Spec.Template.Resources.Limits.Memory != "" {
		resources.Limits[corev1.ResourceMemory] = resource.MustParse(agentDeploy.Spec.Template.Resources.Limits.Memory)
	}

	image := r.imageOrDefault(agentDeploy)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: agentDeploy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(agentDeploy, agentv1.SchemeGroupVersion.WithKind("AgentDeployment")),
			},
			Labels: map[string]string{
				"app":          "agentprimordia",
				"agent-deploy": agentDeploy.Name,
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
							Name:    "agent",
							Image:   image,
							Command: []string{"./ap", "run"},
							Env:     envVars,
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
							Name:    "metrics",
							Image:   image,
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

// ensureService 创建暴露 metrics sidecar 的 Service
func (r *AgentDeploymentReconciler) ensureService(ctx context.Context, agentDeploy *agentv1.AgentDeployment) error {
	svcName := fmt.Sprintf("%s-metrics", agentDeploy.Name)
	var existingSvc corev1.Service

	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: agentDeploy.Namespace}, &existingSvc)
	if err == nil {
		return nil // 已存在
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("检查 Service 失败: %w", err)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: agentDeploy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(agentDeploy, agentv1.SchemeGroupVersion.WithKind("AgentDeployment")),
			},
			Labels: map[string]string{
				"app":          "agentprimordia",
				"agent-deploy": agentDeploy.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":          "agentprimordia",
				"agent-deploy": agentDeploy.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return r.Create(ctx, svc)
}

// ensureHPA 创建或更新 HorizontalPodAutoscaler
func (r *AgentDeploymentReconciler) ensureHPA(ctx context.Context, agentDeploy *agentv1.AgentDeployment) error {
	if agentDeploy.Spec.Autoscaling == nil {
		return nil // 未配置自动扩缩容，跳过
	}

	hpaName := fmt.Sprintf("%s-hpa", agentDeploy.Name)
	deployName := fmt.Sprintf("%s-agent", agentDeploy.Name)
	targetConcurrentTasks := agentDeploy.Spec.Autoscaling.TargetConcurrentTasks

	var existingHPA autoscalingv2.HorizontalPodAutoscaler
	err := r.Get(ctx, types.NamespacedName{Name: hpaName, Namespace: agentDeploy.Namespace}, &existingHPA)
	if err == nil {
		// HPA 已存在，检查是否需要更新副本数或指标
		needsUpdate := false
		if existingHPA.Spec.MinReplicas == nil || *existingHPA.Spec.MinReplicas != agentDeploy.Spec.Autoscaling.MinReplicas {
			existingHPA.Spec.MinReplicas = &agentDeploy.Spec.Autoscaling.MinReplicas
			needsUpdate = true
		}
		if existingHPA.Spec.MaxReplicas != agentDeploy.Spec.Autoscaling.MaxReplicas {
			existingHPA.Spec.MaxReplicas = agentDeploy.Spec.Autoscaling.MaxReplicas
			needsUpdate = true
		}
		// 检查指标目标值是否变化
		if len(existingHPA.Spec.Metrics) > 0 && existingHPA.Spec.Metrics[0].Pods != nil {
			avgVal := existingHPA.Spec.Metrics[0].Pods.Target.AverageValue
			newVal := resource.MustParse(fmt.Sprintf("%d", targetConcurrentTasks))
			if avgVal.Cmp(newVal) != 0 {
				existingHPA.Spec.Metrics[0].Pods.Target.AverageValue = resourcePtr(newVal)
				needsUpdate = true
			}
		}
		if needsUpdate {
			return r.Update(ctx, &existingHPA)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("检查 HPA 失败: %w", err)
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hpaName,
			Namespace: agentDeploy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(agentDeploy, agentv1.SchemeGroupVersion.WithKind("AgentDeployment")),
			},
			Labels: map[string]string{
				"app":          "agentprimordia",
				"agent-deploy": agentDeploy.Name,
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deployName,
			},
			MinReplicas: &agentDeploy.Spec.Autoscaling.MinReplicas,
			MaxReplicas: agentDeploy.Spec.Autoscaling.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.PodsMetricSourceType,
					Pods: &autoscalingv2.PodsMetricSource{
						Metric: autoscalingv2.MetricIdentifier{
							Name: "concurrent_tasks_per_pod",
						},
						Target: autoscalingv2.MetricTarget{
							Type:         autoscalingv2.AverageValueMetricType,
							AverageValue: resourcePtr(resource.MustParse(fmt.Sprintf("%d", targetConcurrentTasks))),
						},
					},
				},
			},
		},
	}

	return r.Create(ctx, hpa)
}

// resourcePtr 返回 resource.Quantity 的指针
func resourcePtr(q resource.Quantity) *resource.Quantity {
	return &q
}

// updateStatus 更新 AgentDeployment 的状态
func (r *AgentDeploymentReconciler) updateStatus(ctx context.Context, agentDeploy *agentv1.AgentDeployment) error {
	deployName := fmt.Sprintf("%s-agent", agentDeploy.Name)
	var deploy appsv1.Deployment

	if err := r.Get(ctx, types.NamespacedName{Name: deployName, Namespace: agentDeploy.Namespace}, &deploy); err != nil {
		return err
	}

	// 查询属于此 AgentDeployment 的 Pod 列表
	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.MatchingLabels{
			"app":          "agentprimordia",
			"agent-deploy": agentDeploy.Name,
		},
		client.InNamespace(agentDeploy.Namespace),
	); err != nil {
		return fmt.Errorf("查询 Pod 列表失败: %w", err)
	}

	// 从 Pod 状态聚合真实指标
	var activeReplicas int32
	var totalRestarts int32
	var completedTasks int64
	var failedTasks int64

	for i := range pods.Items {
		pod := &pods.Items[i]

		// 统计 Running 状态的 Pod 作为活跃副本
		if pod.Status.Phase == corev1.PodRunning {
			activeReplicas++
		}

		// 统计 Failed 阶段的 Pod
		if pod.Status.Phase == corev1.PodFailed {
			failedTasks++
		}

		// 从容器状态统计重启次数
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
		}

		// 从 Pod 条件判断就绪状态（已就绪 = 任务完成）
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				completedTasks++
			}
		}
	}

	// 失败任务也计入重启导致的异常
	failedTasks += int64(totalRestarts)

	agentDeploy.Status.ActiveReplicas = activeReplicas
	agentDeploy.Status.CompletedTasks = completedTasks
	agentDeploy.Status.FailedTasks = failedTasks

	total := agentDeploy.Status.CompletedTasks + agentDeploy.Status.FailedTasks
	if total > 0 {
		agentDeploy.Status.ErrorRate = float64(agentDeploy.Status.FailedTasks) / float64(total)
	} else {
		agentDeploy.Status.ErrorRate = 0
	}

	condition := agentv1.AgentDeploymentCondition{
		Type:               "Available",
		Status:             "True",
		LastTransitionTime: metav1.Now(),
		Reason:             "MinimumReplicasAvailable",
		Message:            fmt.Sprintf("Deployment has %d ready replicas", activeReplicas),
	}

	if activeReplicas < *deploy.Spec.Replicas {
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
		Owns(&corev1.Service{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Complete(r)
}
