/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

// WarmPoolConfig holds warm pool settings from Helm/ConfigMap.
type WarmPoolConfig struct {
	Enabled bool
	Size    int    // target pool size per namespace
	Image   string // agent image (falls back to defaults.DefaultImage)
}

const warmPoolCooldown = 30 * time.Second

// reconcileWarmPool ensures the warm pool has the desired number of idle pods.
// It has a 30s cooldown to avoid excessive API calls.
func (r *AgentTaskReconciler) reconcileWarmPool(ctx context.Context) error {
	r.defaultsMu.RLock()
	cfg := r.defaults.WarmPool
	r.defaultsMu.RUnlock()

	if !cfg.Enabled || cfg.Size <= 0 {
		return nil
	}

	now := time.Now()
	r.defaultsMu.RLock()
	lastCheck := r.warmPoolAt
	r.defaultsMu.RUnlock()

	if now.Sub(lastCheck) < warmPoolCooldown {
		return nil
	}

	r.defaultsMu.Lock()
	r.warmPoolAt = now
	r.defaultsMu.Unlock()

	return r.replenishWarmPool(ctx)
}

// replenishWarmPool creates warm pods to fill the pool to the desired size.
func (r *AgentTaskReconciler) replenishWarmPool(ctx context.Context) error {
	logger := log.FromContext(ctx)

	r.defaultsMu.RLock()
	cfg := r.defaults.WarmPool
	r.defaultsMu.RUnlock()

	if !cfg.Enabled || cfg.Size <= 0 {
		return nil
	}

	// List idle warm pods
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(r.Namespace),
		client.MatchingLabels{
			"hortator.ai/warm-pool":   "true",
			"hortator.ai/warm-status": "idle",
		}); err != nil {
		return fmt.Errorf("list warm pods: %w", err)
	}

	deficit := cfg.Size - len(podList.Items)
	if deficit <= 0 {
		return nil
	}

	logger.Info("Replenishing warm pool", "current", len(podList.Items), "target", cfg.Size, "creating", deficit)

	for i := 0; i < deficit; i++ {
		if _, _, err := r.buildWarmPod(ctx); err != nil {
			return fmt.Errorf("build warm pod %d/%d: %w", i+1, deficit, err)
		}
	}

	return nil
}

// buildWarmPod creates a warm pod and its PVC, returning both.
func (r *AgentTaskReconciler) buildWarmPod(ctx context.Context) (*corev1.Pod, *corev1.PersistentVolumeClaim, error) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	pvcName := fmt.Sprintf("warm-%s-storage", suffix)
	podName := fmt.Sprintf("warm-%s-agent", suffix)

	r.defaultsMu.RLock()
	cfg := r.defaults.WarmPool
	image := cfg.Image
	if image == "" {
		// Use the agentic image for warm pods — it's a superset that can run
		// both the bash (legionary) and Python (centurion/tribune) runtimes.
		image = r.defaults.AgenticImage
		if image == "" {
			image = r.defaults.DefaultImage
		}
	}
	defaults := r.defaults
	r.defaultsMu.RUnlock()

	// Build resource requirements from defaults
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
	if defaults.DefaultRequestsCPU != "" {
		if qty, err := resource.ParseQuantity(defaults.DefaultRequestsCPU); err == nil {
			resources.Requests[corev1.ResourceCPU] = qty
		}
	}
	if defaults.DefaultRequestsMemory != "" {
		if qty, err := resource.ParseQuantity(defaults.DefaultRequestsMemory); err == nil {
			resources.Requests[corev1.ResourceMemory] = qty
		}
	}
	if defaults.DefaultLimitsCPU != "" {
		if qty, err := resource.ParseQuantity(defaults.DefaultLimitsCPU); err == nil {
			resources.Limits[corev1.ResourceCPU] = qty
		}
	}
	if defaults.DefaultLimitsMemory != "" {
		if qty, err := resource.ParseQuantity(defaults.DefaultLimitsMemory); err == nil {
			resources.Limits[corev1.ResourceMemory] = qty
		}
	}

	storageQty, err := parseQuantity("256Mi", "warm pool PVC storage")
	if err != nil {
		return nil, nil, err
	}

	// Create PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: r.Namespace,
			Labels: map[string]string{
				"hortator.ai/warm-pool": "true",
				"hortator.ai/warm-pod":  podName,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageQty,
				},
			},
		},
	}

	if err := r.Create(ctx, pvc); err != nil {
		return nil, nil, fmt.Errorf("create warm PVC: %w", err)
	}

	// Build env vars — try to inject API keys from well-known secrets
	env := []corev1.EnvVar{}

	// Try anthropic-api-key secret
	env = append(env, corev1.EnvVar{
		Name: "ANTHROPIC_API_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "anthropic-api-key"},
				Key:                  "api-key",
				Optional:             boolPtr(true),
			},
		},
	})
	env = append(env, corev1.EnvVar{
		Name: "OPENAI_API_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "openai-api-key"},
				Key:                  "api-key",
				Optional:             boolPtr(true),
			},
		},
	})

	// Create Pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: r.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "hortator-warm",
				"app.kubernetes.io/managed-by": "hortator-operator",
				"hortator.ai/warm-pool":        "true",
				"hortator.ai/warm-status":      "idle",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: workerSpawnSAName, // warm pods use spawn SA since they can be claimed by any tier
			Containers: []corev1.Container{
				{
					Name:      "agent",
					Image:     image,
					Command:   []string{"sh", "-c", `while [ ! -f /inbox/task.json ]; do sleep 0.5; done; TIER=$(cat /inbox/task.json | grep -o '"tier":"[^"]*"' | head -1 | cut -d'"' -f4); case "$TIER" in centurion|tribune) exec python3 /opt/hortator/main.py ;; *) exec /entrypoint.sh ;; esac`},
					Env:       env,
					Resources: resources,
					VolumeMounts: []corev1.VolumeMount{
						{Name: "inbox", MountPath: "/inbox"},
						{Name: "storage", MountPath: "/outbox", SubPath: "outbox"},
						{Name: "storage", MountPath: "/workspace", SubPath: "workspace"},
						{Name: "storage", MountPath: "/memory", SubPath: "memory"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{Name: "inbox", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "storage", VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
				}},
			},
		},
	}

	if err := r.Create(ctx, pod); err != nil {
		// Clean up PVC if pod creation fails
		_ = r.Delete(ctx, pvc)
		return nil, nil, fmt.Errorf("create warm pod: %w", err)
	}

	return pod, pvc, nil
}

// claimWarmPod finds an idle warm pod and claims it for the given task.
func (r *AgentTaskReconciler) claimWarmPod(ctx context.Context, task *corev1alpha1.AgentTask) (*corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(r.Namespace),
		client.MatchingLabels{
			"hortator.ai/warm-pool":   "true",
			"hortator.ai/warm-status": "idle",
		}); err != nil {
		return nil, fmt.Errorf("list warm pods: %w", err)
	}

	if len(podList.Items) == 0 {
		return nil, nil
	}

	pod := &podList.Items[0]

	// Patch pod labels
	pod.Labels["hortator.ai/warm-status"] = "claimed"
	pod.Labels["hortator.ai/task"] = task.Name
	if err := r.Update(ctx, pod); err != nil {
		return nil, fmt.Errorf("claim warm pod: %w", err)
	}

	// Set owner reference on pod
	if err := controllerutil.SetControllerReference(task, pod, r.Scheme); err != nil {
		return nil, fmt.Errorf("set owner ref on pod: %w", err)
	}
	if err := r.Update(ctx, pod); err != nil {
		return nil, fmt.Errorf("update pod owner ref: %w", err)
	}

	// Find and update the associated PVC
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcList, client.InNamespace(r.Namespace),
		client.MatchingLabels{
			"hortator.ai/warm-pool": "true",
			"hortator.ai/warm-pod":  pod.Name,
		}); err != nil {
		return nil, fmt.Errorf("list warm PVCs: %w", err)
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		pvc.Labels["hortator.ai/task"] = task.Name
		if err := controllerutil.SetControllerReference(task, pvc, r.Scheme); err != nil {
			return nil, fmt.Errorf("set owner ref on PVC: %w", err)
		}
		if err := r.Update(ctx, pvc); err != nil {
			return nil, fmt.Errorf("update PVC: %w", err)
		}
	}

	return pod, nil
}

// injectTask writes the task spec JSON into the warm pod's /inbox/task.json via exec.
func (r *AgentTaskReconciler) injectTask(ctx context.Context, task *corev1alpha1.AgentTask, podName string) error {
	taskJSON, err := json.Marshal(task.Spec)
	if err != nil {
		return fmt.Errorf("marshal task spec: %w", err)
	}

	if r.RESTConfig == nil {
		return fmt.Errorf("RESTConfig not set on reconciler")
	}

	req := r.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(task.Namespace).
		SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: "agent",
		Command:   []string{"sh", "-c", "cat > /inbox/task.json"},
		Stdin:     true,
	}, clientgoscheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(r.RESTConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}

	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin: bytes.NewReader(taskJSON),
	})
}

func boolPtr(b bool) *bool {
	return &b
}
