/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func warmReconciler(objs ...client.Object) *AgentTaskReconciler {
	scheme := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(&corev1alpha1.AgentTask{}).Build()
	return &AgentTaskReconciler{
		Client:    fc,
		Scheme:    scheme,
		Namespace: "hortator-system",
		defaults: ClusterDefaults{
			DefaultImage:          "ghcr.io/hortator-ai/agent:latest",
			DefaultTimeout:        600,
			DefaultRequestsCPU:    "100m",
			DefaultRequestsMemory: "128Mi",
			DefaultLimitsCPU:      "500m",
			DefaultLimitsMemory:   "512Mi",
			WarmPool: WarmPoolConfig{
				Enabled: true,
				Size:    2,
			},
		},
	}
}

func TestBuildWarmPod(t *testing.T) {
	ctx := context.Background()
	r := warmReconciler()

	pod, pvc, err := r.buildWarmPod(ctx)
	if err != nil {
		t.Fatalf("buildWarmPod() error: %v", err)
	}

	// Pod labels
	if pod.Labels["hortator.ai/warm-pool"] != "true" {
		t.Error("pod missing warm-pool label")
	}
	if pod.Labels["hortator.ai/warm-status"] != "idle" {
		t.Error("pod missing warm-status=idle label")
	}
	if pod.Labels["app.kubernetes.io/managed-by"] != "hortator-operator" {
		t.Error("pod missing managed-by label")
	}
	if pod.Labels["app.kubernetes.io/name"] != "hortator-warm" {
		t.Error("pod missing app name label")
	}

	// Command override (wait loop)
	if len(pod.Spec.Containers) == 0 {
		t.Fatal("pod has no containers")
	}
	agent := pod.Spec.Containers[0]
	if len(agent.Command) != 3 || agent.Command[0] != "sh" {
		t.Errorf("unexpected command: %v", agent.Command)
	}

	// EmptyDir for /inbox
	foundInbox := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == "inbox" && v.EmptyDir != nil {
			foundInbox = true
		}
	}
	if !foundInbox {
		t.Error("missing EmptyDir volume for inbox")
	}

	// PVC-backed storage volume
	foundStorage := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == "storage" && v.PersistentVolumeClaim != nil {
			foundStorage = true
			if v.PersistentVolumeClaim.ClaimName != pvc.Name {
				t.Errorf("storage PVC claim name = %q, want %q", v.PersistentVolumeClaim.ClaimName, pvc.Name)
			}
		}
	}
	if !foundStorage {
		t.Error("missing PVC volume for storage")
	}

	// Volume mounts for /outbox, /workspace, /memory
	mountPaths := map[string]bool{}
	for _, m := range agent.VolumeMounts {
		mountPaths[m.MountPath] = true
	}
	for _, path := range []string{"/inbox", "/outbox", "/workspace", "/memory"} {
		if !mountPaths[path] {
			t.Errorf("missing volume mount for %s", path)
		}
	}

	// Default image
	if agent.Image != "ghcr.io/hortator-ai/agent:latest" {
		t.Errorf("image = %q, want default", agent.Image)
	}

	// RestartPolicy
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("RestartPolicy = %q, want Never", pod.Spec.RestartPolicy)
	}

	// ServiceAccountName
	if pod.Spec.ServiceAccountName != "hortator-worker" {
		t.Errorf("ServiceAccountName = %q, want hortator-worker", pod.Spec.ServiceAccountName)
	}

	// No init containers
	if len(pod.Spec.InitContainers) != 0 {
		t.Errorf("warm pod should have no init containers, got %d", len(pod.Spec.InitContainers))
	}

	// PVC labels
	if pvc.Labels["hortator.ai/warm-pool"] != "true" {
		t.Error("PVC missing warm-pool label")
	}
	if pvc.Labels["hortator.ai/warm-pod"] != pod.Name {
		t.Errorf("PVC warm-pod label = %q, want %q", pvc.Labels["hortator.ai/warm-pod"], pod.Name)
	}
}

func TestClaimWarmPod_NoPodsAvailable(t *testing.T) {
	ctx := context.Background()
	r := warmReconciler()

	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{Name: "task-1", Namespace: "hortator-system"},
		Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
	}

	pod, err := r.claimWarmPod(ctx, task)
	if err != nil {
		t.Fatalf("claimWarmPod() error: %v", err)
	}
	if pod != nil {
		t.Error("expected nil pod when no warm pods available")
	}
}

func TestClaimWarmPod_IdlePodExists(t *testing.T) {
	ctx := context.Background()

	warmPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "warm-123-agent",
			Namespace: "hortator-system",
			Labels: map[string]string{
				"hortator.ai/warm-pool":   "true",
				"hortator.ai/warm-status": "idle",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "agent", Image: "test"}},
		},
	}
	warmPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "warm-123-storage",
			Namespace: "hortator-system",
			Labels: map[string]string{
				"hortator.ai/warm-pool": "true",
				"hortator.ai/warm-pod":  "warm-123-agent",
			},
		},
	}

	r := warmReconciler(warmPod, warmPVC)

	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{Name: "task-1", Namespace: "hortator-system"},
		Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
	}

	pod, err := r.claimWarmPod(ctx, task)
	if err != nil {
		t.Fatalf("claimWarmPod() error: %v", err)
	}
	if pod == nil {
		t.Fatal("expected a claimed pod")
	}

	// Verify labels updated
	claimed := &corev1.Pod{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(pod), claimed); err != nil {
		t.Fatalf("get claimed pod: %v", err)
	}
	if claimed.Labels["hortator.ai/warm-status"] != "claimed" {
		t.Errorf("warm-status = %q, want claimed", claimed.Labels["hortator.ai/warm-status"])
	}
	if claimed.Labels["hortator.ai/task"] != "task-1" {
		t.Errorf("task label = %q, want task-1", claimed.Labels["hortator.ai/task"])
	}
}

func TestClaimWarmPod_OnlyClaimedPods(t *testing.T) {
	ctx := context.Background()

	claimedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "warm-456-agent",
			Namespace: "hortator-system",
			Labels: map[string]string{
				"hortator.ai/warm-pool":   "true",
				"hortator.ai/warm-status": "claimed",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "agent", Image: "test"}},
		},
	}

	r := warmReconciler(claimedPod)

	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{Name: "task-2", Namespace: "hortator-system"},
		Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
	}

	pod, err := r.claimWarmPod(ctx, task)
	if err != nil {
		t.Fatalf("claimWarmPod() error: %v", err)
	}
	if pod != nil {
		t.Error("expected nil pod when only claimed pods exist")
	}
}

func TestReconcileWarmPool_CreatesPodsWhenEmpty(t *testing.T) {
	ctx := context.Background()
	r := warmReconciler()
	// Reset cooldown
	r.warmPoolAt = r.warmPoolAt.Add(-warmPoolCooldown * 2)

	if err := r.reconcileWarmPool(ctx); err != nil {
		t.Fatalf("reconcileWarmPool() error: %v", err)
	}

	// Should have created 2 warm pods
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace("hortator-system"),
		client.MatchingLabels{"hortator.ai/warm-pool": "true"}); err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(podList.Items) != 2 {
		t.Errorf("created %d warm pods, want 2", len(podList.Items))
	}
}

func TestReconcileWarmPool_CreatesOneMoreWhenPartial(t *testing.T) {
	ctx := context.Background()

	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "warm-existing-agent",
			Namespace: "hortator-system",
			Labels: map[string]string{
				"hortator.ai/warm-pool":   "true",
				"hortator.ai/warm-status": "idle",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "agent", Image: "test"}},
		},
	}

	r := warmReconciler(existingPod)
	r.warmPoolAt = r.warmPoolAt.Add(-warmPoolCooldown * 2)

	if err := r.reconcileWarmPool(ctx); err != nil {
		t.Fatalf("reconcileWarmPool() error: %v", err)
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace("hortator-system"),
		client.MatchingLabels{"hortator.ai/warm-pool": "true", "hortator.ai/warm-status": "idle"}); err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(podList.Items) != 2 {
		t.Errorf("total idle warm pods = %d, want 2", len(podList.Items))
	}
}

func TestReconcileWarmPool_DoesNothingWhenFull(t *testing.T) {
	ctx := context.Background()

	pods := []client.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "warm-a-agent", Namespace: "hortator-system",
				Labels: map[string]string{"hortator.ai/warm-pool": "true", "hortator.ai/warm-status": "idle"}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: "test"}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "warm-b-agent", Namespace: "hortator-system",
				Labels: map[string]string{"hortator.ai/warm-pool": "true", "hortator.ai/warm-status": "idle"}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent", Image: "test"}}},
		},
	}

	r := warmReconciler(pods...)
	r.warmPoolAt = r.warmPoolAt.Add(-warmPoolCooldown * 2)

	if err := r.reconcileWarmPool(ctx); err != nil {
		t.Fatalf("reconcileWarmPool() error: %v", err)
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace("hortator-system"),
		client.MatchingLabels{"hortator.ai/warm-pool": "true"}); err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(podList.Items) != 2 {
		t.Errorf("total warm pods = %d, want 2 (no new ones)", len(podList.Items))
	}
}

func TestReconcileWarmPool_DisabledDoesNothing(t *testing.T) {
	ctx := context.Background()
	r := warmReconciler()
	r.defaults.WarmPool.Enabled = false

	if err := r.reconcileWarmPool(ctx); err != nil {
		t.Fatalf("reconcileWarmPool() error: %v", err)
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace("hortator-system"),
		client.MatchingLabels{"hortator.ai/warm-pool": "true"}); err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(podList.Items) != 0 {
		t.Errorf("should not create pods when disabled, got %d", len(podList.Items))
	}
}

func TestWarmPoolConfig_FromConfigMap(t *testing.T) {
	scheme := newTestScheme()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "hortator-config", Namespace: "hortator-system"},
		Data: map[string]string{
			"warmPoolEnabled": "true",
			"warmPoolSize":    "5",
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	r := &AgentTaskReconciler{Client: fc, Scheme: scheme, Namespace: "hortator-system"}

	r.loadClusterDefaults(context.Background())

	if !r.defaults.WarmPool.Enabled {
		t.Error("WarmPool.Enabled should be true")
	}
	if r.defaults.WarmPool.Size != 5 {
		t.Errorf("WarmPool.Size = %d, want 5", r.defaults.WarmPool.Size)
	}
}

func TestWarmPoolConfig_DefaultSize(t *testing.T) {
	scheme := newTestScheme()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "hortator-config", Namespace: "hortator-system"},
		Data: map[string]string{
			"warmPoolEnabled": "true",
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	r := &AgentTaskReconciler{Client: fc, Scheme: scheme, Namespace: "hortator-system"}

	r.loadClusterDefaults(context.Background())

	if r.defaults.WarmPool.Size != 2 {
		t.Errorf("WarmPool.Size = %d, want 2 (default)", r.defaults.WarmPool.Size)
	}
}
