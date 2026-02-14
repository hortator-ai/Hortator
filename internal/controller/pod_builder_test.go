/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func defaultReconciler(scheme *runtime.Scheme, objs ...runtime.Object) *AgentTaskReconciler {
	fc := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
	return &AgentTaskReconciler{
		Client: fc,
		Scheme: scheme,
		defaults: ClusterDefaults{
			DefaultImage:          "ghcr.io/hortator-ai/hortator/agent:latest",
			DefaultTimeout:        600,
			DefaultRequestsCPU:    "100m",
			DefaultRequestsMemory: "128Mi",
			DefaultLimitsCPU:      "500m",
			DefaultLimitsMemory:   "512Mi",
		},
	}
}

func TestBuildPod(t *testing.T) {
	scheme := newTestScheme()

	t.Run("default image from cluster defaults", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "hello"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pod.Spec.Containers[0].Image != "ghcr.io/hortator-ai/hortator/agent:latest" {
			t.Errorf("image = %q, want default", pod.Spec.Containers[0].Image)
		}
	})

	t.Run("custom image from task spec", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "hello", Image: "myimage:v1"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pod.Spec.Containers[0].Image != "myimage:v1" {
			t.Errorf("image = %q, want myimage:v1", pod.Spec.Containers[0].Image)
		}
	})

	t.Run("env vars include standard hortator vars", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "my-task", Namespace: "test-ns"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "do stuff"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envMap := envToMap(pod.Spec.Containers[0].Env)
		if envMap["HORTATOR_PROMPT"] != "do stuff" {
			t.Errorf("HORTATOR_PROMPT = %q", envMap["HORTATOR_PROMPT"])
		}
		if envMap["HORTATOR_TASK_NAME"] != "my-task" {
			t.Errorf("HORTATOR_TASK_NAME = %q", envMap["HORTATOR_TASK_NAME"])
		}
		if envMap["HORTATOR_TASK_NAMESPACE"] != "test-ns" {
			t.Errorf("HORTATOR_TASK_NAMESPACE = %q", envMap["HORTATOR_TASK_NAMESPACE"])
		}
	})

	t.Run("capabilities env var", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Capabilities: []string{"shell", "web-fetch"}},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envMap := envToMap(pod.Spec.Containers[0].Env)
		if envMap["HORTATOR_CAPABILITIES"] != "shell,web-fetch" {
			t.Errorf("HORTATOR_CAPABILITIES = %q", envMap["HORTATOR_CAPABILITIES"])
		}
	})

	t.Run("anthropic api key env var", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec: corev1alpha1.AgentTaskSpec{
				Prompt: "test",
				Model: &corev1alpha1.ModelSpec{
					Endpoint:  "https://api.anthropic.com/v1",
					Name:      "claude-sonnet",
					ApiKeyRef: &corev1alpha1.SecretKeyRef{SecretName: "my-secret", Key: "api-key"},
				},
			},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		found := false
		for _, e := range pod.Spec.Containers[0].Env {
			if e.Name == "ANTHROPIC_API_KEY" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
				if e.ValueFrom.SecretKeyRef.Name == "my-secret" && e.ValueFrom.SecretKeyRef.Key == "api-key" {
					found = true
				}
			}
		}
		if !found {
			t.Error("expected ANTHROPIC_API_KEY env from secret")
		}
	})

	t.Run("openai api key env var", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec: corev1alpha1.AgentTaskSpec{
				Prompt: "test",
				Model: &corev1alpha1.ModelSpec{
					Endpoint:  "https://api.openai.com/v1",
					ApiKeyRef: &corev1alpha1.SecretKeyRef{SecretName: "oai", Key: "key"},
				},
			},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		found := false
		for _, e := range pod.Spec.Containers[0].Env {
			if e.Name == "OPENAI_API_KEY" {
				found = true
			}
		}
		if !found {
			t.Error("expected OPENAI_API_KEY env")
		}
	})

	t.Run("generic api key env var", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec: corev1alpha1.AgentTaskSpec{
				Prompt: "test",
				Model: &corev1alpha1.ModelSpec{
					Endpoint:  "http://ollama:11434/v1",
					ApiKeyRef: &corev1alpha1.SecretKeyRef{SecretName: "s", Key: "k"},
				},
			},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		found := false
		for _, e := range pod.Spec.Containers[0].Env {
			if e.Name == "LLM_API_KEY" {
				found = true
			}
		}
		if !found {
			t.Error("expected LLM_API_KEY env")
		}
	})

	t.Run("custom env vars direct and secretRef", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec: corev1alpha1.AgentTaskSpec{
				Prompt: "test",
				Env: []corev1alpha1.EnvVar{
					{Name: "MY_VAR", Value: "my-value"},
					{Name: "SECRET_VAR", ValueFrom: &corev1alpha1.EnvVarSource{
						SecretKeyRef: &corev1alpha1.SecretKeyRef{SecretName: "s", Key: "k"},
					}},
				},
			},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envMap := envToMap(pod.Spec.Containers[0].Env)
		if envMap["MY_VAR"] != "my-value" {
			t.Errorf("MY_VAR = %q", envMap["MY_VAR"])
		}
		// SECRET_VAR should be from secret ref (value will be empty in map)
		foundSecret := false
		for _, e := range pod.Spec.Containers[0].Env {
			if e.Name == "SECRET_VAR" && e.ValueFrom != nil {
				foundSecret = true
			}
		}
		if !foundSecret {
			t.Error("expected SECRET_VAR from secretRef")
		}
	})

	t.Run("init container uses busybox with env var", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pod.Spec.InitContainers) != 1 {
			t.Fatalf("expected 1 init container, got %d", len(pod.Spec.InitContainers))
		}
		init := pod.Spec.InitContainers[0]
		if init.Image != "busybox:1.37.0" {
			t.Errorf("init image = %q", init.Image)
		}
		foundTaskJSON := false
		for _, e := range init.Env {
			if e.Name == "TASK_JSON" {
				foundTaskJSON = true
			}
		}
		if !foundTaskJSON {
			t.Error("expected TASK_JSON env in init container")
		}
	})

	t.Run("presidio endpoint injected when enabled", func(t *testing.T) {
		r := defaultReconciler(scheme)
		r.defaults.PresidioEnabled = true
		r.defaults.PresidioEndpoint = "http://presidio:8080"
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envMap := envToMap(pod.Spec.Containers[0].Env)
		if envMap["PRESIDIO_ENDPOINT"] != "http://presidio:8080" {
			t.Errorf("PRESIDIO_ENDPOINT = %q", envMap["PRESIDIO_ENDPOINT"])
		}
	})

	t.Run("volumes include inbox and storage", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		volNames := map[string]bool{}
		for _, v := range pod.Spec.Volumes {
			volNames[v.Name] = true
		}
		if !volNames["inbox-ephemeral"] {
			t.Error("missing inbox-ephemeral volume")
		}
		if !volNames["storage"] {
			t.Error("missing storage volume")
		}
	})

	t.Run("volume mounts for standard paths", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		mountPaths := map[string]bool{}
		for _, m := range pod.Spec.Containers[0].VolumeMounts {
			mountPaths[m.MountPath] = true
		}
		for _, path := range []string{"/inbox", "/outbox", "/workspace", "/memory"} {
			if !mountPaths[path] {
				t.Errorf("missing mount path %s", path)
			}
		}
	})

	t.Run("service account basic for pod without spawn", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Capabilities: []string{"shell", "web-fetch"}},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pod.Spec.ServiceAccountName != "hortator-worker-basic" {
			t.Errorf("ServiceAccountName = %q, want hortator-worker-basic", pod.Spec.ServiceAccountName)
		}
	})

	t.Run("service account spawn for pod with spawn capability", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Capabilities: []string{"shell", "spawn"}},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pod.Spec.ServiceAccountName != "hortator-worker-spawn" {
			t.Errorf("ServiceAccountName = %q, want hortator-worker-spawn", pod.Spec.ServiceAccountName)
		}
	})

	t.Run("service account spawn for agentic tier (auto-injected spawn)", func(t *testing.T) {
		r := defaultReconciler(scheme)
		r.defaults.AgenticImage = "ghcr.io/hortator-ai/hortator/agentic:latest"
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Tier: "tribune"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pod.Spec.ServiceAccountName != "hortator-worker-spawn" {
			t.Errorf("ServiceAccountName = %q, want hortator-worker-spawn", pod.Spec.ServiceAccountName)
		}
	})

	t.Run("service account basic for pod with no capabilities", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pod.Spec.ServiceAccountName != "hortator-worker-basic" {
			t.Errorf("ServiceAccountName = %q, want hortator-worker-basic", pod.Spec.ServiceAccountName)
		}
	})

	t.Run("shell policy env vars injected from AgentPolicy", func(t *testing.T) {
		policy := &corev1alpha1.AgentPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1alpha1.AgentPolicySpec{
				AllowedShellCommands: []string{"python", "node"},
				DeniedShellCommands:  []string{"rm", "curl"},
			},
		}
		r := defaultReconciler(scheme, policy)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envMap := envToMap(pod.Spec.Containers[0].Env)
		if envMap["HORTATOR_ALLOWED_COMMANDS"] != "python,node" {
			t.Errorf("HORTATOR_ALLOWED_COMMANDS = %q, want 'python,node'", envMap["HORTATOR_ALLOWED_COMMANDS"])
		}
		if envMap["HORTATOR_DENIED_COMMANDS"] != "rm,curl" {
			t.Errorf("HORTATOR_DENIED_COMMANDS = %q, want 'rm,curl'", envMap["HORTATOR_DENIED_COMMANDS"])
		}
	})

	t.Run("readOnlyWorkspace sets workspace mount to read-only", func(t *testing.T) {
		policy := &corev1alpha1.AgentPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: corev1alpha1.AgentPolicySpec{
				ReadOnlyWorkspace: true,
			},
		}
		r := defaultReconciler(scheme, policy)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, m := range pod.Spec.Containers[0].VolumeMounts {
			if m.MountPath == "/workspace" {
				if !m.ReadOnly {
					t.Error("/workspace should be read-only")
				}
				return
			}
		}
		t.Error("/workspace mount not found")
	})

	t.Run("pod labels", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pod.Labels["hortator.ai/task"] != "t1" {
			t.Errorf("task label = %q", pod.Labels["hortator.ai/task"])
		}
		if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
			t.Errorf("restart policy = %v", pod.Spec.RestartPolicy)
		}
	})
}

func TestEnsurePVC(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()

	intPtr := func(v int) *int { return &v }

	t.Run("creates PVC with 256Mi for legionary", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default", UID: "uid1"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Tier: "legionary"},
		}
		if err := r.ensurePVC(ctx, task); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client_key("default", "t1-storage"), pvc); err != nil {
			t.Fatalf("PVC not found: %v", err)
		}
		qty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if qty.String() != "256Mi" {
			t.Errorf("storage = %s, want 256Mi", qty.String())
		}
	})

	t.Run("creates PVC with 1Gi for tribune", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t2", Namespace: "default", UID: "uid2"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Tier: "tribune"},
		}
		if err := r.ensurePVC(ctx, task); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client_key("default", "t2-storage"), pvc); err != nil {
			t.Fatalf("PVC not found: %v", err)
		}
		qty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if qty.String() != "1Gi" {
			t.Errorf("storage = %s, want 1Gi", qty.String())
		}
	})

	t.Run("custom size from spec", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t3", Namespace: "default", UID: "uid3"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Storage: &corev1alpha1.StorageSpec{Size: "5Gi"}},
		}
		if err := r.ensurePVC(ctx, task); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client_key("default", "t3-storage"), pvc); err != nil {
			t.Fatalf("PVC not found: %v", err)
		}
		qty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if qty.String() != "5Gi" {
			t.Errorf("storage = %s, want 5Gi", qty.String())
		}
	})

	t.Run("retention annotation", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t4", Namespace: "default", UID: "uid4"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Storage: &corev1alpha1.StorageSpec{RetainDays: intPtr(30)}},
		}
		if err := r.ensurePVC(ctx, task); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client_key("default", "t4-storage"), pvc); err != nil {
			t.Fatalf("PVC not found: %v", err)
		}
		if pvc.Annotations["hortator.ai/retention"] != "30d" {
			t.Errorf("retention = %q, want 30d", pvc.Annotations["hortator.ai/retention"])
		}
	})

	t.Run("custom storage class", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t5", Namespace: "default", UID: "uid5"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Storage: &corev1alpha1.StorageSpec{StorageClass: "fast-ssd"}},
		}
		if err := r.ensurePVC(ctx, task); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client_key("default", "t5-storage"), pvc); err != nil {
			t.Fatalf("PVC not found: %v", err)
		}
		if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "fast-ssd" {
			t.Errorf("storageClass = %v, want fast-ssd", pvc.Spec.StorageClassName)
		}
	})

	t.Run("doesnt create duplicate", func(t *testing.T) {
		existingPVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "t6-storage", Namespace: "default"},
		}
		r := defaultReconciler(scheme, existingPVC)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t6", Namespace: "default", UID: "uid6"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		if err := r.ensurePVC(ctx, task); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid size returns error", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t7", Namespace: "default", UID: "uid7"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Storage: &corev1alpha1.StorageSpec{Size: "notasize"}},
		}
		err := r.ensurePVC(ctx, task)
		if err == nil {
			t.Error("expected error for invalid size")
		}
	})
}

func TestBuildVolumes(t *testing.T) {
	scheme := newTestScheme()
	r := defaultReconciler(scheme)

	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{Name: "my-task", Namespace: "default"},
		Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
	}

	volumes, _ := r.buildVolumes(task)
	found := false
	for _, v := range volumes {
		if v.Name == "storage" && v.PersistentVolumeClaim != nil {
			if v.PersistentVolumeClaim.ClaimName == "my-task-storage" {
				found = true
			}
		}
	}
	if !found {
		t.Error("storage volume should reference my-task-storage PVC")
	}
}

func TestBuildResourcesPartialSpec(t *testing.T) {
	scheme := newTestScheme()
	r := defaultReconciler(scheme)

	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Resources: &corev1alpha1.ResourceRequirements{
				Requests: &corev1alpha1.ResourceList{CPU: "200m"},
			},
		},
	}

	res, err := r.buildResources(task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Requests.Cpu().String() != "200m" {
		t.Errorf("CPU request = %s, want 200m", res.Requests.Cpu().String())
	}
	if res.Limits != nil {
		t.Errorf("Limits should be nil for partial spec, got %v", res.Limits)
	}
}

func TestEnsureWorkerRBAC(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()

	t.Run("creates all three SA sets in namespace", func(t *testing.T) {
		r := defaultReconciler(scheme)
		if err := r.ensureWorkerRBAC(ctx, "test-ns"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, name := range []string{"hortator-worker-basic", "hortator-worker-spawn", "hortator-worker"} {
			sa := &corev1.ServiceAccount{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: name}, sa); err != nil {
				t.Fatalf("ServiceAccount %s not found: %v", name, err)
			}
			if sa.Labels["app.kubernetes.io/managed-by"] != "hortator-operator" {
				t.Errorf("SA %s label = %q, want hortator-operator", name, sa.Labels["app.kubernetes.io/managed-by"])
			}

			role := &rbacv1.Role{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: name}, role); err != nil {
				t.Fatalf("Role %s not found: %v", name, err)
			}
			if len(role.Rules) != 2 {
				t.Errorf("Role %s: expected 2 rules, got %d", name, len(role.Rules))
			}

			rb := &rbacv1.RoleBinding{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: name}, rb); err != nil {
				t.Fatalf("RoleBinding %s not found: %v", name, err)
			}
			if rb.RoleRef.Name != name {
				t.Errorf("RoleBinding %s: RoleRef.Name = %q", name, rb.RoleRef.Name)
			}
		}
	})

	t.Run("basic role has no create verb", func(t *testing.T) {
		r := defaultReconciler(scheme)
		if err := r.ensureWorkerRBAC(ctx, "verb-ns"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		role := &rbacv1.Role{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: "verb-ns", Name: "hortator-worker-basic"}, role); err != nil {
			t.Fatalf("Role not found: %v", err)
		}
		for _, verb := range role.Rules[0].Verbs {
			if verb == "create" {
				t.Error("basic role should not have create verb on agenttasks")
			}
		}
	})

	t.Run("spawn role has create verb", func(t *testing.T) {
		r := defaultReconciler(scheme)
		if err := r.ensureWorkerRBAC(ctx, "verb-ns2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		role := &rbacv1.Role{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: "verb-ns2", Name: "hortator-worker-spawn"}, role); err != nil {
			t.Fatalf("Role not found: %v", err)
		}
		hasCreate := false
		for _, verb := range role.Rules[0].Verbs {
			if verb == "create" {
				hasCreate = true
			}
		}
		if !hasCreate {
			t.Error("spawn role should have create verb on agenttasks")
		}
	})

	t.Run("idempotent — no error on second call", func(t *testing.T) {
		r := defaultReconciler(scheme)
		if err := r.ensureWorkerRBAC(ctx, "idempotent-ns"); err != nil {
			t.Fatalf("first call: %v", err)
		}
		if err := r.ensureWorkerRBAC(ctx, "idempotent-ns"); err != nil {
			t.Fatalf("second call: %v", err)
		}
	})

	t.Run("skips creation when resources already exist", func(t *testing.T) {
		existingSA := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "hortator-worker", Namespace: "existing-ns"},
		}
		existingRole := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "hortator-worker", Namespace: "existing-ns"},
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{"core.hortator.ai"}, Resources: []string{"agenttasks"}, Verbs: []string{"get"}}},
		}
		existingRB := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "hortator-worker", Namespace: "existing-ns"},
			RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "hortator-worker"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "hortator-worker", Namespace: "existing-ns"}},
		}
		r := defaultReconciler(scheme, existingSA, existingRole, existingRB)
		if err := r.ensureWorkerRBAC(ctx, "existing-ns"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestBuildPodShellCommandPolicy(t *testing.T) {
	scheme := newTestScheme()

	t.Run("AllowedShellCommands injects env var", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t-shell", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		policy := corev1alpha1.AgentPolicy{
			Spec: corev1alpha1.AgentPolicySpec{
				AllowedShellCommands: []string{"ls", "cat", "grep"},
				DeniedShellCommands:  []string{"rm", "curl"},
			},
		}
		pod, err := r.buildPod(task, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envMap := envToMap(pod.Spec.Containers[0].Env)
		if envMap["HORTATOR_ALLOWED_COMMANDS"] != "ls,cat,grep" {
			t.Errorf("expected allowed commands 'ls,cat,grep', got %q", envMap["HORTATOR_ALLOWED_COMMANDS"])
		}
		if envMap["HORTATOR_DENIED_COMMANDS"] != "rm,curl" {
			t.Errorf("expected denied commands 'rm,curl', got %q", envMap["HORTATOR_DENIED_COMMANDS"])
		}
	})

	t.Run("ReadOnlyWorkspace sets mount to read-only", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t-ro", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "analyze"},
		}
		policy := corev1alpha1.AgentPolicy{
			Spec: corev1alpha1.AgentPolicySpec{
				ReadOnlyWorkspace: true,
			},
		}
		pod, err := r.buildPod(task, policy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		found := false
		for _, m := range pod.Spec.Containers[0].VolumeMounts {
			if m.MountPath == "/workspace" {
				found = true
				if !m.ReadOnly {
					t.Error("expected /workspace to be read-only")
				}
			}
		}
		if !found {
			t.Error("/workspace mount not found")
		}
	})

	t.Run("no policies means no shell env vars", func(t *testing.T) {
		r := defaultReconciler(scheme)
		task := &corev1alpha1.AgentTask{
			ObjectMeta: metav1.ObjectMeta{Name: "t-nopol", Namespace: "default"},
			Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
		}
		pod, err := r.buildPod(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envMap := envToMap(pod.Spec.Containers[0].Env)
		if _, ok := envMap["HORTATOR_ALLOWED_COMMANDS"]; ok {
			t.Error("unexpected HORTATOR_ALLOWED_COMMANDS without policy")
		}
		if _, ok := envMap["HORTATOR_DENIED_COMMANDS"]; ok {
			t.Error("unexpected HORTATOR_DENIED_COMMANDS without policy")
		}
	})
}

// helpers

func envToMap(envs []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envs))
	for _, e := range envs {
		if e.ValueFrom == nil {
			m[e.Name] = e.Value
		}
	}
	return m
}

func client_key(ns, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: ns, Name: name}
}
