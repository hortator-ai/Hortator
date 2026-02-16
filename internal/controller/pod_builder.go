/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
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

	// Skip owner reference if the task has retain-pvc annotation, so the PVC
	// survives cascade deletion when the AgentTask is removed.
	if task.Annotations == nil || task.Annotations["hortator.ai/retain-pvc"] != "true" {
		if err := controllerutil.SetControllerReference(task, pvc, r.Scheme); err != nil {
			return err
		}
	}

	return r.Create(ctx, pvc)
}

const (
	workerServiceAccountName = "hortator-worker"
	workerRoleName           = "hortator-worker"
	workerRoleBindingName    = "hortator-worker"

	// Per-capability RBAC service accounts
	workerBasicSAName = "hortator-worker-basic"
	workerSpawnSAName = "hortator-worker-spawn"
)

// ensureWorkerRBAC creates per-capability ServiceAccounts, Roles, and
// RoleBindings in the task's namespace if they don't already exist:
//   - hortator-worker-basic: read-only AgentTask access + status updates
//   - hortator-worker-spawn: basic + create AgentTasks
//   - hortator-worker: legacy backward-compatible SA (spawn permissions)
func (r *AgentTaskReconciler) ensureWorkerRBAC(ctx context.Context, namespace string) error {
	logger := log.FromContext(ctx)

	type rbacSet struct {
		name  string
		verbs []string // verbs for agenttasks resource
	}

	sets := []rbacSet{
		{name: workerBasicSAName, verbs: []string{"get", "list", "watch"}},
		{name: workerSpawnSAName, verbs: []string{"get", "list", "watch", "create"}},
		{name: workerServiceAccountName, verbs: []string{"get", "list", "create", "update"}}, // legacy
	}

	for _, s := range sets {
		// ServiceAccount
		sa := &corev1.ServiceAccount{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: s.name}, sa); err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to check worker ServiceAccount %s: %w", s.name, err)
			}
			sa = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s.name,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "hortator-operator",
						"app.kubernetes.io/name":       "hortator-worker",
					},
				},
			}
			if err := r.Create(ctx, sa); err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create worker ServiceAccount %s in %s: %w", s.name, namespace, err)
			}
			logger.Info("Created worker ServiceAccount", "name", s.name, "namespace", namespace)
		}

		// Role
		role := &rbacv1.Role{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: s.name}, role); err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to check worker Role %s: %w", s.name, err)
			}
			role = &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s.name,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "hortator-operator",
						"app.kubernetes.io/name":       "hortator-worker",
					},
				},
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{"core.hortator.ai"},
						Resources: []string{"agenttasks"},
						Verbs:     s.verbs,
					},
					{
						APIGroups: []string{"core.hortator.ai"},
						Resources: []string{"agenttasks/status"},
						Verbs:     []string{"get", "update", "patch"},
					},
				},
			}
			if err := r.Create(ctx, role); err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create worker Role %s in %s: %w", s.name, namespace, err)
			}
			logger.Info("Created worker Role", "name", s.name, "namespace", namespace)
		}

		// RoleBinding
		rb := &rbacv1.RoleBinding{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: s.name}, rb); err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("failed to check worker RoleBinding %s: %w", s.name, err)
			}
			rb = &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s.name,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "hortator-operator",
						"app.kubernetes.io/name":       "hortator-worker",
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "Role",
					Name:     s.name,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      s.name,
						Namespace: namespace,
					},
				},
			}
			if err := r.Create(ctx, rb); err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create worker RoleBinding %s in %s: %w", s.name, namespace, err)
			}
			logger.Info("Created worker RoleBinding", "name", s.name, "namespace", namespace)
		}
	}

	return nil
}

// isAgenticTier returns true if the tier uses the Python agentic runtime.
func isAgenticTier(tier string) bool {
	return tier == "tribune" || tier == "centurion"
}

// workerSAForCaps returns the appropriate ServiceAccount name based on
// effective capabilities. Pods with "spawn" get hortator-worker-spawn;
// all others get hortator-worker-basic.
func workerSAForCaps(caps []string) string {
	for _, c := range caps {
		if c == "spawn" {
			return workerSpawnSAName
		}
	}
	return workerBasicSAName
}

// buildPod creates a pod spec for the agent task.
func (r *AgentTaskReconciler) buildPod(task *corev1alpha1.AgentTask, policies ...corev1alpha1.AgentPolicy) (*corev1.Pod, error) {
	image := task.Spec.Image
	if image == "" {
		if isAgenticTier(task.Spec.Tier) {
			image = r.defaults.AgenticImage
		} else {
			image = r.defaults.DefaultImage
		}
	}

	// Build effective capabilities: auto-inject "spawn" for tribune/centurion tiers
	// so they always have access to task delegation tools.
	effectiveCaps := make([]string, len(task.Spec.Capabilities))
	copy(effectiveCaps, task.Spec.Capabilities)
	if isAgenticTier(task.Spec.Tier) {
		hasSpawn := false
		for _, c := range effectiveCaps {
			if c == "spawn" {
				hasSpawn = true
				break
			}
		}
		if !hasSpawn {
			effectiveCaps = append(effectiveCaps, "spawn")
		}
	}

	env := []corev1.EnvVar{
		{Name: "HORTATOR_PROMPT", Value: task.Spec.Prompt},
		{Name: "HORTATOR_TASK_NAME", Value: task.Name},
		{Name: "HORTATOR_TASK_NAMESPACE", Value: task.Namespace},
		{Name: "HORTATOR_TIER", Value: task.Spec.Tier},
		{Name: "HORTATOR_ROLE", Value: task.Spec.Role},
	}

	if len(effectiveCaps) > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "HORTATOR_CAPABILITIES",
			Value: strings.Join(effectiveCaps, ","),
		})
	}

	// Determine effective maxIterations (task spec > tier default)
	maxIter := 1 // legionary default
	if task.Spec.MaxIterations != nil {
		maxIter = *task.Spec.MaxIterations
	} else {
		switch task.Spec.Tier {
		case "tribune":
			maxIter = 5
		case "centurion":
			maxIter = 3
		}
	}
	env = append(env, corev1.EnvVar{
		Name:  "HORTATOR_MAX_ITERATIONS",
		Value: fmt.Sprintf("%d", maxIter),
	})

	// Inject iteration count for reincarnated agents
	env = append(env, corev1.EnvVar{
		Name:  "HORTATOR_ITERATION",
		Value: fmt.Sprintf("%d", task.Status.Attempts+1),
	})

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

	// Shell command filtering from AgentPolicy
	readOnlyWorkspace := false
	var allAllowedCmds, allDeniedCmds []string
	for _, policy := range policies {
		if len(policy.Spec.AllowedShellCommands) > 0 {
			allAllowedCmds = append(allAllowedCmds, policy.Spec.AllowedShellCommands...)
		}
		if len(policy.Spec.DeniedShellCommands) > 0 {
			allDeniedCmds = append(allDeniedCmds, policy.Spec.DeniedShellCommands...)
		}
		if policy.Spec.ReadOnlyWorkspace {
			readOnlyWorkspace = true
		}
	}
	if len(allAllowedCmds) > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "HORTATOR_ALLOWED_COMMANDS",
			Value: strings.Join(allAllowedCmds, ","),
		})
	}
	if len(allDeniedCmds) > 0 {
		env = append(env, corev1.EnvVar{
			Name:  "HORTATOR_DENIED_COMMANDS",
			Value: strings.Join(allDeniedCmds, ","),
		})
	}

	resources, err := r.buildResources(task)
	if err != nil {
		return nil, fmt.Errorf("invalid resource spec: %w", err)
	}

	volumes, volumeMounts := r.buildVolumes(task)

	// Apply ReadOnlyWorkspace policy: make /workspace mount read-only
	if readOnlyWorkspace {
		for i := range volumeMounts {
			if volumeMounts[i].MountPath == "/workspace" {
				volumeMounts[i].ReadOnly = true
				break
			}
		}
	}

	// Discover and mount retained PVCs for knowledge discovery.
	retainedPVCs := r.mountRetainedPVCs(task, &volumes, &volumeMounts)

	// Init container writes task.json via env var to avoid shell interpolation.
	// We wrap the spec in a map that includes taskId (the CR name) so runtimes
	// can identify themselves without relying solely on env vars.
	taskPayload := map[string]any{
		"taskId": task.Name,
	}
	specJSON, err := json.Marshal(task.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task spec: %w", err)
	}
	// Merge spec fields into the payload so existing consumers (prompt, tier, etc.) still work
	var specMap map[string]any
	if err := json.Unmarshal(specJSON, &specMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task spec: %w", err)
	}
	for k, v := range specMap {
		taskPayload[k] = v
	}
	taskSpecJSON, err := json.Marshal(taskPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task payload: %w", err)
	}

	// Build context.json with prior work references (from retained PVCs)
	contextJSON := r.buildContextJSON(retainedPVCs)

	// Select the correct volume for the init container's /inbox mount
	inboxVolumeName := "inbox-ephemeral"
	if isAgenticTier(task.Spec.Tier) {
		inboxVolumeName = "storage"
	}
	inboxMount := corev1.VolumeMount{Name: inboxVolumeName, MountPath: "/inbox"}
	if isAgenticTier(task.Spec.Tier) {
		inboxMount.SubPath = "inbox"
	}

	initScript := `mkdir -p /inbox && printf '%s' "$TASK_JSON" > /inbox/task.json`
	if contextJSON != "" {
		initScript += ` && printf '%s' "$CONTEXT_JSON" > /inbox/context.json`
	}

	initEnv := []corev1.EnvVar{
		{Name: "TASK_JSON", Value: string(taskSpecJSON)},
	}
	if contextJSON != "" {
		initEnv = append(initEnv, corev1.EnvVar{Name: "CONTEXT_JSON", Value: contextJSON})
	}

	// Deliver input files to /inbox (base64-decode each file)
	for i, f := range task.Spec.InputFiles {
		envName := fmt.Sprintf("INPUT_FILE_%d", i)
		initEnv = append(initEnv, corev1.EnvVar{Name: envName, Value: f.Data})
		// Use base64 -d to decode the file data
		initScript += fmt.Sprintf(` && printf '%%s' "$%s" | base64 -d > /inbox/%s`, envName, f.Filename)
	}

	initContainers := []corev1.Container{
		{
			Name:         "write-task-json",
			Image:        "busybox:1.37.0",
			Command:      []string{"sh", "-c", initScript},
			Env:          initEnv,
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
		// Anonymizer may run on a separate port/endpoint
		anonymizerEndpoint := r.defaults.PresidioAnonymizerEndpoint
		if anonymizerEndpoint == "" {
			// Default: same host, port 3001
			anonymizerEndpoint = strings.TrimSuffix(r.defaults.PresidioEndpoint, "/analyze")
			// Replace port 3000 with 3001 if present
			anonymizerEndpoint = strings.Replace(anonymizerEndpoint, ":3000", ":3001", 1)
		}
		env = append(env, corev1.EnvVar{
			Name:  "PRESIDIO_ANONYMIZER_ENDPOINT",
			Value: anonymizerEndpoint,
		})
	}

	// Build capability labels for NetworkPolicy matching
	podLabels := map[string]string{
		"app.kubernetes.io/name":       "hortator-agent",
		"app.kubernetes.io/instance":   task.Name,
		"app.kubernetes.io/managed-by": "hortator-operator",
		"hortator.ai/task":             task.Name,
		"hortator.ai/managed":          "true",
		"hortator.ai/tier":             task.Spec.Tier,
	}
	for _, cap := range effectiveCaps {
		podLabels[fmt.Sprintf("hortator.ai/cap-%s", cap)] = "true"
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-agent", task.Name),
			Namespace: task.Namespace,
			Labels:    podLabels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: workerSAForCaps(effectiveCaps),
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

// mountRetainedPVCs discovers retained PVCs and adds them as read-only volume
// mounts at /prior/<task-name>/ for knowledge discovery.
func (r *AgentTaskReconciler) mountRetainedPVCs(task *corev1alpha1.AgentTask,
	volumes *[]corev1.Volume, mounts *[]corev1.VolumeMount) []RetainedPVC {

	r.defaultsMu.RLock()
	cfg := r.defaults.StorageRetained
	r.defaultsMu.RUnlock()

	if cfg.Discovery == "none" || !cfg.AutoMount {
		return nil
	}

	retained, err := r.discoverRetainedPVCs(context.Background(), task, cfg)
	if err != nil || len(retained) == 0 {
		return nil
	}

	// Limit to 5 to avoid pod spec bloat
	maxMounts := 5
	if len(retained) > maxMounts {
		retained = retained[:maxMounts]
	}

	for i, rpvc := range retained {
		volName := fmt.Sprintf("prior-%d", i)
		*volumes = append(*volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: rpvc.Name,
					ReadOnly:  true,
				},
			},
		})
		mountName := rpvc.TaskName
		if mountName == "" {
			mountName = rpvc.Name
		}
		*mounts = append(*mounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: fmt.Sprintf("/prior/%s", mountName),
			ReadOnly:  true,
		})
	}

	return retained
}

// buildContextJSON creates a JSON string for /inbox/context.json containing
// references to mounted prior work PVCs.
func (r *AgentTaskReconciler) buildContextJSON(retained []RetainedPVC) string {
	if len(retained) == 0 {
		return ""
	}

	type priorEntry struct {
		TaskName    string   `json:"taskName"`
		MountPath   string   `json:"mountPath"`
		Tags        []string `json:"tags"`
		CompletedAt string   `json:"completedAt,omitempty"`
		Reason      string   `json:"reason,omitempty"`
	}

	entries := make([]priorEntry, 0, len(retained))
	for _, rpvc := range retained {
		name := rpvc.TaskName
		if name == "" {
			name = rpvc.Name
		}
		entries = append(entries, priorEntry{
			TaskName:    name,
			MountPath:   fmt.Sprintf("/prior/%s", name),
			Tags:        rpvc.Tags,
			CompletedAt: rpvc.CompletedAt,
			Reason:      rpvc.Reason,
		})
	}

	ctx := map[string]interface{}{
		"prior_work": entries,
	}
	data, err := json.Marshal(ctx)
	if err != nil {
		return ""
	}
	return string(data)
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
