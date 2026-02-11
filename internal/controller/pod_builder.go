/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

// ensurePVC creates a PVC for the task if it doesn't already exist.
// All tiers get persistent storage so agents can produce artifacts.
func (r *AgentTaskReconciler) ensurePVC(ctx context.Context, task *corev1alpha1.AgentTask) error {
	pvcName := fmt.Sprintf("%s-storage", task.Name)

	existing := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Namespace: task.Namespace, Name: pvcName}, existing)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	size := "256Mi"
	if task.Spec.Tier == "centurion" || task.Spec.Tier == "tribune" {
		size = "1Gi"
	}
	if task.Spec.Storage != nil && task.Spec.Storage.Size != "" {
		size = task.Spec.Storage.Size
	}

	annotations := map[string]string{}
	if task.Spec.Storage != nil && task.Spec.Storage.RetainDays != nil {
		annotations["hortator.ai/retention"] = fmt.Sprintf("%dd", *task.Spec.Storage.RetainDays)
	}

	storageQty, err := parseQuantity(size, "PVC storage size")
	if err != nil {
		return err
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: task.Namespace,
			Labels: map[string]string{
				"hortator.ai/task": task.Name,
			},
			Annotations: annotations,
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

	if task.Spec.Storage != nil && task.Spec.Storage.StorageClass != "" {
		pvc.Spec.StorageClassName = &task.Spec.Storage.StorageClass
	}

	if err := controllerutil.SetControllerReference(task, pvc, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, pvc)
}

// isAgenticTier returns true if the tier uses the Python agentic runtime.
func isAgenticTier(tier string) bool {
	return tier == "tribune" || tier == "centurion"
}

// buildPod creates a pod spec for the agent task.
func (r *AgentTaskReconciler) buildPod(task *corev1alpha1.AgentTask) (*corev1.Pod, error) {
	image := task.Spec.Image
	if image == "" {
		if isAgenticTier(task.Spec.Tier) {
			image = r.defaults.AgenticImage
		} else {
			image = r.defaults.DefaultImage
		}
	}

	env := []corev1.EnvVar{
		{Name: "HORTATOR_PROMPT", Value: task.Spec.Prompt},
		{Name: "HORTATOR_TASK_NAME", Value: task.Name},
		{Name: "HORTATOR_TASK_NAMESPACE", Value: task.Namespace},
		{Name: "HORTATOR_TIER", Value: task.Spec.Tier},
		{Name: "HORTATOR_ROLE", Value: task.Spec.Role},
	}

	if len(task.Spec.Capabilities) > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "HORTATOR_CAPABILITIES",
			Value: strings.Join(task.Spec.Capabilities, ","),
		})
	}

	if task.Spec.Model != nil && task.Spec.Model.Name != "" {
		env = append(env, corev1.EnvVar{
			Name:  "HORTATOR_MODEL",
			Value: task.Spec.Model.Name,
		})
	}

	// Inject API key from model.apiKeyRef
	if task.Spec.Model != nil && task.Spec.Model.ApiKeyRef != nil {
		apiKeyEnvName := "LLM_API_KEY"
		if task.Spec.Model.Endpoint != "" {
			endpoint := strings.ToLower(task.Spec.Model.Endpoint)
			if strings.Contains(endpoint, "anthropic") {
				apiKeyEnvName = "ANTHROPIC_API_KEY"
			} else if strings.Contains(endpoint, "openai") {
				apiKeyEnvName = "OPENAI_API_KEY"
			}
		}
		env = append(env, corev1.EnvVar{
			Name: apiKeyEnvName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: task.Spec.Model.ApiKeyRef.SecretName,
					},
					Key: task.Spec.Model.ApiKeyRef.Key,
				},
			},
		})
	}

	// Add custom env vars
	for _, e := range task.Spec.Env {
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			env = append(env, corev1.EnvVar{
				Name: e.Name,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: e.ValueFrom.SecretKeyRef.SecretName,
						},
						Key: e.ValueFrom.SecretKeyRef.Key,
					},
				},
			})
		} else {
			env = append(env, corev1.EnvVar{
				Name:  e.Name,
				Value: e.Value,
			})
		}
	}

	resources, err := r.buildResources(task)
	if err != nil {
		return nil, fmt.Errorf("invalid resource spec: %w", err)
	}

	volumes, volumeMounts := r.buildVolumes(task)

	// Init container writes task.json via env var to avoid shell interpolation.
	taskSpecJSON, err := json.Marshal(task.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task spec: %w", err)
	}

	// Select the correct volume for the init container's /inbox mount
	inboxVolumeName := "inbox-ephemeral"
	if isAgenticTier(task.Spec.Tier) {
		inboxVolumeName = "storage"
	}
	inboxMount := corev1.VolumeMount{Name: inboxVolumeName, MountPath: "/inbox"}
	if isAgenticTier(task.Spec.Tier) {
		inboxMount.SubPath = "inbox"
	}

	initContainers := []corev1.Container{
		{
			Name:    "write-task-json",
			Image:   "busybox:1.37.0",
			Command: []string{"sh", "-c", `mkdir -p /inbox && printf '%s' "$TASK_JSON" > /inbox/task.json`},
			Env: []corev1.EnvVar{
				{Name: "TASK_JSON", Value: string(taskSpecJSON)},
			},
			VolumeMounts: []corev1.VolumeMount{inboxMount},
		},
	}

	// Presidio PII detection — centralized service
	presidioEnabled := r.defaults.PresidioEnabled || task.Spec.Presidio != nil
	if presidioEnabled && r.defaults.PresidioEndpoint != "" {
		env = append(env, corev1.EnvVar{
			Name:  "PRESIDIO_ENDPOINT",
			Value: r.defaults.PresidioEndpoint,
		})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-agent", task.Name),
			Namespace: task.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "hortator-agent",
				"app.kubernetes.io/instance":   task.Name,
				"app.kubernetes.io/managed-by": "hortator-operator",
				"hortator.ai/task":             task.Name,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: "hortator-worker",
			InitContainers:     initContainers,
			Containers: []corev1.Container{
				{
					Name:         "agent",
					Image:        image,
					Env:          env,
					Resources:    resources,
					VolumeMounts: volumeMounts,
				},
			},
			Volumes: volumes,
		},
	}

	return pod, nil
}

// buildVolumes returns volumes and volume mounts for the agent pod.
// Agentic tiers (tribune/centurion) mount /inbox from PVC so child results
// persist across reincarnations. Legionaries use EmptyDir for /inbox.
func (r *AgentTaskReconciler) buildVolumes(task *corev1alpha1.AgentTask) ([]corev1.Volume, []corev1.VolumeMount) {
	pvcName := fmt.Sprintf("%s-storage", task.Name)

	volumes := []corev1.Volume{
		{Name: "inbox-ephemeral", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "storage", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
		}},
	}

	mounts := []corev1.VolumeMount{
		{Name: "storage", MountPath: "/outbox", SubPath: "outbox"},
		{Name: "storage", MountPath: "/workspace", SubPath: "workspace"},
		{Name: "storage", MountPath: "/memory", SubPath: "memory"},
	}

	if isAgenticTier(task.Spec.Tier) {
		// Agentic tiers: /inbox on PVC so child-results/ survives reincarnation.
		// The init container writes task.json here too (via PVC subpath).
		mounts = append(mounts, corev1.VolumeMount{
			Name: "storage", MountPath: "/inbox", SubPath: "inbox",
		})
	} else {
		// Legionaries: ephemeral /inbox is fine — single run, no reincarnation.
		mounts = append(mounts, corev1.VolumeMount{
			Name: "inbox-ephemeral", MountPath: "/inbox",
		})
	}

	return volumes, mounts
}

// parseQuantity parses a resource string, returning a clean error instead of panicking.
func parseQuantity(value, label string) (resource.Quantity, error) {
	qty, err := resource.ParseQuantity(value)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("invalid %s %q: %w", label, value, err)
	}
	return qty, nil
}

// buildResources constructs resource requirements from the task spec or cluster defaults.
func (r *AgentTaskReconciler) buildResources(task *corev1alpha1.AgentTask) (corev1.ResourceRequirements, error) {
	resources := corev1.ResourceRequirements{}

	if task.Spec.Resources != nil {
		if task.Spec.Resources.Requests != nil {
			resources.Requests = corev1.ResourceList{}
			if task.Spec.Resources.Requests.CPU != "" {
				qty, err := parseQuantity(task.Spec.Resources.Requests.CPU, "CPU request")
				if err != nil {
					return resources, err
				}
				resources.Requests[corev1.ResourceCPU] = qty
			}
			if task.Spec.Resources.Requests.Memory != "" {
				qty, err := parseQuantity(task.Spec.Resources.Requests.Memory, "memory request")
				if err != nil {
					return resources, err
				}
				resources.Requests[corev1.ResourceMemory] = qty
			}
		}
		if task.Spec.Resources.Limits != nil {
			resources.Limits = corev1.ResourceList{}
			if task.Spec.Resources.Limits.CPU != "" {
				qty, err := parseQuantity(task.Spec.Resources.Limits.CPU, "CPU limit")
				if err != nil {
					return resources, err
				}
				resources.Limits[corev1.ResourceCPU] = qty
			}
			if task.Spec.Resources.Limits.Memory != "" {
				qty, err := parseQuantity(task.Spec.Resources.Limits.Memory, "memory limit")
				if err != nil {
					return resources, err
				}
				resources.Limits[corev1.ResourceMemory] = qty
			}
		}
	} else {
		resources.Requests = corev1.ResourceList{}
		resources.Limits = corev1.ResourceList{}
		if r.defaults.DefaultRequestsCPU != "" {
			qty, err := parseQuantity(r.defaults.DefaultRequestsCPU, "default CPU request")
			if err != nil {
				return resources, err
			}
			resources.Requests[corev1.ResourceCPU] = qty
		}
		if r.defaults.DefaultRequestsMemory != "" {
			qty, err := parseQuantity(r.defaults.DefaultRequestsMemory, "default memory request")
			if err != nil {
				return resources, err
			}
			resources.Requests[corev1.ResourceMemory] = qty
		}
		if r.defaults.DefaultLimitsCPU != "" {
			qty, err := parseQuantity(r.defaults.DefaultLimitsCPU, "default CPU limit")
			if err != nil {
				return resources, err
			}
			resources.Limits[corev1.ResourceCPU] = qty
		}
		if r.defaults.DefaultLimitsMemory != "" {
			qty, err := parseQuantity(r.defaults.DefaultLimitsMemory, "default memory limit")
			if err != nil {
				return resources, err
			}
			resources.Limits[corev1.ResourceMemory] = qty
		}
	}

	return resources, nil
}
