package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

func TestLoadClusterDefaults_WithConfigMap(t *testing.T) {
	scheme := newTestScheme()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "hortator-config", Namespace: "hortator-system"},
		Data: map[string]string{
			"defaultTimeout":        "900",
			"defaultImage":          "custom-image:v2",
			"defaultRequestsCPU":    "200m",
			"defaultRequestsMemory": "256Mi",
			"defaultLimitsCPU":      "1",
			"defaultLimitsMemory":   "1Gi",
			"presidioEnabled":       "true",
			"presidioEndpoint":      "http://presidio:8080",
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	r := &AgentTaskReconciler{Client: fc, Scheme: scheme, Namespace: "hortator-system"}

	r.loadClusterDefaults(context.Background())

	if r.defaults.DefaultTimeout != 900 {
		t.Errorf("DefaultTimeout = %d, want 900", r.defaults.DefaultTimeout)
	}
	if r.defaults.DefaultImage != "custom-image:v2" {
		t.Errorf("DefaultImage = %q", r.defaults.DefaultImage)
	}
	if r.defaults.DefaultRequestsCPU != "200m" {
		t.Errorf("DefaultRequestsCPU = %q", r.defaults.DefaultRequestsCPU)
	}
	if !r.defaults.PresidioEnabled {
		t.Error("PresidioEnabled should be true")
	}
	if r.defaults.PresidioEndpoint != "http://presidio:8080" {
		t.Errorf("PresidioEndpoint = %q", r.defaults.PresidioEndpoint)
	}
}

func TestLoadClusterDefaults_WithoutConfigMap(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentTaskReconciler{Client: fc, Scheme: scheme, Namespace: "hortator-system"}

	r.loadClusterDefaults(context.Background())

	if r.defaults.DefaultTimeout != 600 {
		t.Errorf("DefaultTimeout = %d, want 600", r.defaults.DefaultTimeout)
	}
	if r.defaults.DefaultRequestsCPU != "100m" {
		t.Errorf("DefaultRequestsCPU = %q, want 100m", r.defaults.DefaultRequestsCPU)
	}
	if r.defaults.DefaultLimitsMemory != "512Mi" {
		t.Errorf("DefaultLimitsMemory = %q, want 512Mi", r.defaults.DefaultLimitsMemory)
	}
}

func TestLoadClusterDefaults_PartialConfigMap(t *testing.T) {
	scheme := newTestScheme()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "hortator-config", Namespace: "hortator-system"},
		Data: map[string]string{
			"defaultTimeout": "1200",
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	r := &AgentTaskReconciler{Client: fc, Scheme: scheme, Namespace: "hortator-system"}

	r.loadClusterDefaults(context.Background())

	if r.defaults.DefaultTimeout != 1200 {
		t.Errorf("DefaultTimeout = %d, want 1200", r.defaults.DefaultTimeout)
	}
	// Rest should be defaults
	if r.defaults.DefaultRequestsCPU != "100m" {
		t.Errorf("DefaultRequestsCPU = %q, want 100m", r.defaults.DefaultRequestsCPU)
	}
}

func TestLoadClusterDefaults_EnvOverride(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentTaskReconciler{Client: fc, Scheme: scheme, Namespace: "hortator-system"}

	t.Setenv("HORTATOR_DEFAULT_AGENT_IMAGE", "env-image:v3")
	r.loadClusterDefaults(context.Background())

	if r.defaults.DefaultImage != "env-image:v3" {
		t.Errorf("DefaultImage = %q, want env-image:v3", r.defaults.DefaultImage)
	}
}

func TestRefreshDefaultsIfStale(t *testing.T) {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).Build()

	t.Run("fresh cache no reload", func(t *testing.T) {
		r := &AgentTaskReconciler{
			Client:      fc,
			Scheme:      scheme,
			Namespace:   "hortator-system",
			defaultsTTL: 60 * time.Second,
		}
		r.defaults = ClusterDefaults{DefaultTimeout: 999}
		r.defaultsAt = time.Now()

		r.refreshDefaultsIfStale(context.Background())

		if r.defaults.DefaultTimeout != 999 {
			t.Error("should not have reloaded fresh cache")
		}
	})

	t.Run("stale cache triggers reload", func(t *testing.T) {
		r := &AgentTaskReconciler{
			Client:      fc,
			Scheme:      scheme,
			Namespace:   "hortator-system",
			defaultsTTL: 1 * time.Millisecond,
		}
		r.defaults = ClusterDefaults{DefaultTimeout: 999}
		r.defaultsAt = time.Now().Add(-1 * time.Second)

		r.refreshDefaultsIfStale(context.Background())

		if r.defaults.DefaultTimeout != 600 {
			t.Errorf("should have reloaded, got timeout = %d", r.defaults.DefaultTimeout)
		}
	})
}

func TestCollectPodLogs_NilClientset(t *testing.T) {
	r := &AgentTaskReconciler{Clientset: nil}
	result := r.collectPodLogs(context.Background(), "ns", "pod")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestNotifyParentTask(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()

	t.Run("no parent is no-op", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		// Should not panic
		r.notifyParentTask(ctx, task)
	})

	t.Run("already in childTasks is no-op", func(t *testing.T) {
		parent := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "parent", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "parent"},
			Status:     corev1alpha1.AgentTaskStatus{ChildTasks: []string{"child-1"}},
		}
		fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(parent).WithStatusSubresource(parent).Build()
		r := &AgentTaskReconciler{Client: fc, Scheme: scheme}

		child := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "child-1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "child", ParentTaskID: "parent"},
		}
		r.notifyParentTask(ctx, child)

		// Parent should still have exactly one child
		updated := &corev1alpha1.AgentTask{}
		_ = fc.Get(ctx, client_key("default", "parent"), updated)
		if len(updated.Status.ChildTasks) != 1 {
			t.Errorf("childTasks = %v, want [child-1]", updated.Status.ChildTasks)
		}
	})
}
