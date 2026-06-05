package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1 "agentprimordia/operator/api/v1"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = agentv1.AddToScheme(s)
	return s
}

func makeAgentDeployment(name, namespace string, replicas int32) *agentv1.AgentDeployment {
	return &agentv1.AgentDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: agentv1.AgentDeploymentSpec{
			Replicas: replicas,
			Template: agentv1.AgentTemplateSpec{
				Provider:     "openai",
				Model:        "gpt-4o",
				SystemPrompt: "test prompt",
				MaxTurns:     5,
			},
		},
	}
}

func TestReconcile_CreatesConfigMapAndDeployment(t *testing.T) {
	scheme := newScheme()
	ad := makeAgentDeployment("test-agent", "default", 1)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ad).
		WithStatusSubresource(&agentv1.AgentDeployment{}).
		Build()

	r := &AgentDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-agent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify ConfigMap was created
	cm := &corev1.ConfigMap{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-agent-config", Namespace: "default"}, cm); err != nil {
		t.Errorf("ConfigMap should be created: %v", err)
	}
	if cm.Data["ap.yaml"] == "" {
		t.Error("ConfigMap should contain ap.yaml key")
	}

	// Verify Deployment was created
	deploy := &appsv1.Deployment{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-agent-agent", Namespace: "default"}, deploy); err != nil {
		t.Errorf("Deployment should be created: %v", err)
	}
	if *deploy.Spec.Replicas != 1 {
		t.Errorf("Deployment replicas = %d, want 1", *deploy.Spec.Replicas)
	}
}

func TestReconcile_HandlesMissingAgentDeployment(t *testing.T) {
	scheme := newScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	r := &AgentDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	result, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	})
	if err != nil {
		t.Errorf("Reconcile should not error on missing resource: %v", err)
	}
	if result.Requeue {
		t.Error("Should not requeue for missing resource")
	}
}

func TestReconcile_ConfigMapExistsSkipsCreation(t *testing.T) {
	scheme := newScheme()
	ad := makeAgentDeployment("test-agent", "default", 1)

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-config", Namespace: "default"},
		Data:       map[string]string{"ap.yaml": "existing"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ad, existingCM).
		WithStatusSubresource(&agentv1.AgentDeployment{}).
		Build()

	r := &AgentDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-agent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	cm := &corev1.ConfigMap{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-agent-config", Namespace: "default"}, cm)
	if cm.Data["ap.yaml"] != "existing" {
		t.Error("Existing ConfigMap should not be overwritten")
	}
}

func TestReconcile_DeploymentExistsUpdatesReplicas(t *testing.T) {
	scheme := newScheme()
	ad := makeAgentDeployment("test-agent", "default", 3)

	replicas := int32(1)
	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-agent", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agentprimordia"}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ad, existingDeploy).
		WithStatusSubresource(&agentv1.AgentDeployment{}).
		Build()

	r := &AgentDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-agent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-agent-agent", Namespace: "default"}, deploy)
	if *deploy.Spec.Replicas != 3 {
		t.Errorf("Deployment replicas = %d, want 3", *deploy.Spec.Replicas)
	}
}

func TestReconcile_UpdatesStatus(t *testing.T) {
	scheme := newScheme()
	ad := makeAgentDeployment("test-agent", "default", 2)
	ad.ResourceVersion = "1"

	replicas := int32(2)
	readyReplicas := int32(2)
	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-agent", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agentprimordia"}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: readyReplicas},
	}

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-config", Namespace: "default"},
		Data:       map[string]string{"ap.yaml": "test"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ad, existingDeploy, existingCM).
		WithStatusSubresource(&agentv1.AgentDeployment{}).
		Build()

	r := &AgentDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-agent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	updated := &agentv1.AgentDeployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-agent", Namespace: "default"}, updated)
	if updated.Status.ActiveReplicas != 2 {
		t.Errorf("ActiveReplicas = %d, want 2", updated.Status.ActiveReplicas)
	}
	if len(updated.Status.Conditions) != 1 {
		t.Fatalf("Conditions length = %d, want 1", len(updated.Status.Conditions))
	}
	if updated.Status.Conditions[0].Type != "Available" {
		t.Errorf("Condition Type = %s, want Available", updated.Status.Conditions[0].Type)
	}
	if updated.Status.Conditions[0].Status != "True" {
		t.Errorf("Condition Status = %s, want True", updated.Status.Conditions[0].Status)
	}
}

func TestReconcile_StatusUnavailableWhenReplicasBelowTarget(t *testing.T) {
	scheme := newScheme()
	ad := makeAgentDeployment("test-agent", "default", 3)
	ad.ResourceVersion = "1"

	replicas := int32(3)
	readyReplicas := int32(1)
	existingDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-agent", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agentprimordia"}},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: readyReplicas},
	}

	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent-config", Namespace: "default"},
		Data:       map[string]string{"ap.yaml": "test"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ad, existingDeploy, existingCM).
		WithStatusSubresource(&agentv1.AgentDeployment{}).
		Build()

	r := &AgentDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-agent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	updated := &agentv1.AgentDeployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "test-agent", Namespace: "default"}, updated)
	if updated.Status.Conditions[0].Status != "False" {
		t.Errorf("Condition Status = %s, want False (replicas below target)", updated.Status.Conditions[0].Status)
	}
	if updated.Status.Conditions[0].Reason != "MinimumReplicasUnavailable" {
		t.Errorf("Condition Reason = %s, want MinimumReplicasUnavailable", updated.Status.Conditions[0].Reason)
	}
}

func TestReconcile_APISecretRefInEnv(t *testing.T) {
	scheme := newScheme()
	ad := makeAgentDeployment("secret-agent", "default", 1)
	ad.Spec.Template.APISecretRef = "my-api-secret"

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ad).
		WithStatusSubresource(&agentv1.AgentDeployment{}).
		Build()

	r := &AgentDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "secret-agent", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	deploy := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "secret-agent-agent", Namespace: "default"}, deploy)

	found := false
	for _, env := range deploy.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "OPENAI_API_KEY" && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef.Name == "my-api-secret" {
			found = true
		}
	}
	if !found {
		t.Error("OPENAI_API_KEY env should reference my-api-secret")
	}
}

// Verify OwnerReferences are set on created resources
func TestReconcile_OwnerReferencesSet(t *testing.T) {
	scheme := newScheme()
	ad := makeAgentDeployment("owned", "default", 1)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ad).
		WithStatusSubresource(&agentv1.AgentDeployment{}).
		Build()

	r := &AgentDeploymentReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "owned", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	cm := &corev1.ConfigMap{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "owned-config", Namespace: "default"}, cm)
	if len(cm.OwnerReferences) != 1 || cm.OwnerReferences[0].Name != "owned" {
		t.Error("ConfigMap should have OwnerReference to AgentDeployment")
	}

	deploy := &appsv1.Deployment{}
	_ = fakeClient.Get(context.Background(), types.NamespacedName{Name: "owned-agent", Namespace: "default"}, deploy)
	if len(deploy.OwnerReferences) != 1 || deploy.OwnerReferences[0].Name != "owned" {
		t.Error("Deployment should have OwnerReference to AgentDeployment")
	}
}

// Compile-time check: AgentDeploymentReconciler implements client.Object
var _ client.Object = (*agentv1.AgentDeployment)(nil)
