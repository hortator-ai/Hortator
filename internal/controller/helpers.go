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
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

// updateStatusWithRetry wraps Status().Update() with retry.RetryOnConflict to handle
// optimistic concurrency conflicts. On conflict, it re-fetches the latest version of
// the AgentTask and re-applies the desired status before retrying.
func (r *AgentTaskReconciler) updateStatusWithRetry(ctx context.Context, task *corev1alpha1.AgentTask) error {
	// Capture the desired status before retrying
	desiredStatus := task.Status.DeepCopy()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the latest version (skip on first attempt since task is already fresh)
		latest := &corev1alpha1.AgentTask{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: task.Namespace,
			Name:      task.Name,
		}, latest); err != nil {
			if errors.IsNotFound(err) {
				return nil // Task was deleted, nothing to update
			}
			return err
		}

		// Apply the desired status onto the latest version
		latest.Status = *desiredStatus

		// Update with the latest resource version
		if err := r.Status().Update(ctx, latest); err != nil {
			return err
		}

		// Sync the resource version back so the caller has a fresh copy
		task.ResourceVersion = latest.ResourceVersion
		return nil
	})
}

// refreshDefaultsIfStale reloads cluster defaults only if the cache TTL has expired.
func (r *AgentTaskReconciler) refreshDefaultsIfStale(ctx context.Context) {
	ttl := r.defaultsTTL
	if ttl == 0 {
		ttl = defaultConfigCacheTTL
	}
	r.defaultsMu.RLock()
	fresh := time.Since(r.defaultsAt) < ttl
	r.defaultsMu.RUnlock()
	if fresh {
		return
	}
	r.loadClusterDefaults(ctx)
}

// loadClusterDefaults reads the hortator-config ConfigMap and caches defaults.
func (r *AgentTaskReconciler) loadClusterDefaults(ctx context.Context) {
	ns := r.Namespace
	if ns == "" {
		ns = "hortator-system"
	}

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: "hortator-config"}, cm)
	defaultImage := os.Getenv("HORTATOR_DEFAULT_AGENT_IMAGE")
	if defaultImage == "" {
		defaultImage = "ghcr.io/hortator-ai/agent:latest"
	}
	agenticImage := os.Getenv("HORTATOR_AGENTIC_IMAGE")
	if agenticImage == "" {
		agenticImage = "ghcr.io/hortator-ai/agent-agentic:latest"
	}

	if err != nil {
		r.defaultsMu.Lock()
		r.defaults = ClusterDefaults{
			DefaultTimeout:        600,
			DefaultImage:          defaultImage,
			AgenticImage:          agenticImage,
			DefaultRequestsCPU:    "100m",
			DefaultRequestsMemory: "128Mi",
			DefaultLimitsCPU:      "500m",
			DefaultLimitsMemory:   "512Mi",
		}
		r.defaultsAt = time.Now()
		r.defaultsMu.Unlock()
		return
	}

	d := ClusterDefaults{
		DefaultTimeout:        600,
		DefaultImage:          defaultImage,
		AgenticImage:          agenticImage,
		DefaultRequestsCPU:    "100m",
		DefaultRequestsMemory: "128Mi",
		DefaultLimitsCPU:      "500m",
		DefaultLimitsMemory:   "512Mi",
	}

	if v, ok := cm.Data["defaultTimeout"]; ok {
		if t, err := strconv.Atoi(v); err == nil {
			d.DefaultTimeout = t
		}
	}
	if v, ok := cm.Data["defaultImage"]; ok && v != "" {
		d.DefaultImage = v
	}
	if v, ok := cm.Data["agenticImage"]; ok && v != "" {
		d.AgenticImage = v
	}
	if v, ok := cm.Data["defaultRequestsCPU"]; ok && v != "" {
		d.DefaultRequestsCPU = v
	}
	if v, ok := cm.Data["defaultRequestsMemory"]; ok && v != "" {
		d.DefaultRequestsMemory = v
	}
	if v, ok := cm.Data["defaultLimitsCPU"]; ok && v != "" {
		d.DefaultLimitsCPU = v
	}
	if v, ok := cm.Data["defaultLimitsMemory"]; ok && v != "" {
		d.DefaultLimitsMemory = v
	}
	if v, ok := cm.Data["enforceNamespaceLabels"]; ok {
		d.EnforceNamespaceLabels = v == "true"
	}
	if v, ok := cm.Data["presidioEnabled"]; ok {
		d.PresidioEnabled = v == "true"
	}
	if v, ok := cm.Data["presidioEndpoint"]; ok && v != "" {
		d.PresidioEndpoint = v
	}
	if v, ok := cm.Data["warmPoolEnabled"]; ok {
		d.WarmPool.Enabled = v == "true"
	}
	if v, ok := cm.Data["warmPoolSize"]; ok {
		if size, err := strconv.Atoi(v); err == nil {
			d.WarmPool.Size = size
		}
	}
	if d.WarmPool.Size == 0 {
		d.WarmPool.Size = 2
	}
	if v, ok := cm.Data["resultCacheEnabled"]; ok {
		d.ResultCacheEnabled = v == "true"
	}
	if v, ok := cm.Data["resultCacheTTLSeconds"]; ok {
		if ttl, err := strconv.Atoi(v); err == nil {
			d.ResultCacheTTL = time.Duration(ttl) * time.Second
		}
	}
	if v, ok := cm.Data["resultCacheMaxEntries"]; ok {
		if max, err := strconv.Atoi(v); err == nil {
			d.ResultCacheMaxEntries = max
		}
	}

	// ── Budget config ──────────────────────────────────────────────────────
	if v, ok := cm.Data["budgetEnabled"]; ok {
		d.Budget.Enabled = v == "true"
	}
	if v, ok := cm.Data["budgetDefaultMaxCostUsd"]; ok && v != "" {
		d.Budget.DefaultMaxCostUsd = v
	}
	if v, ok := cm.Data["budgetWarningPercent"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.Budget.WarningPercent = n
		}
	}
	if d.Budget.WarningPercent == 0 {
		d.Budget.WarningPercent = 80
	}
	if v, ok := cm.Data["budgetSoftCeilingAction"]; ok && v != "" {
		d.Budget.SoftCeilingAction = v
	}
	if d.Budget.SoftCeilingAction == "" {
		d.Budget.SoftCeilingAction = "winddown"
	}
	if v, ok := cm.Data["budgetGraceMaxLLMCalls"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.Budget.GraceMaxLLMCalls = n
		}
	}
	if d.Budget.GraceMaxLLMCalls == 0 {
		d.Budget.GraceMaxLLMCalls = 3
	}
	if v, ok := cm.Data["budgetGraceMaxSeconds"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.Budget.GraceMaxSeconds = n
		}
	}
	if d.Budget.GraceMaxSeconds == 0 {
		d.Budget.GraceMaxSeconds = 60
	}
	if v, ok := cm.Data["budgetPriceSource"]; ok && v != "" {
		d.Budget.PriceSource = v
	}
	if d.Budget.PriceSource == "" {
		d.Budget.PriceSource = "litellm"
	}
	if v, ok := cm.Data["budgetRefreshIntervalHours"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.Budget.RefreshIntervalH = n
		}
	}
	if d.Budget.RefreshIntervalH == 0 {
		d.Budget.RefreshIntervalH = 24
	}
	if v, ok := cm.Data["budgetFallbackBehavior"]; ok && v != "" {
		d.Budget.FallbackBehavior = v
	}
	if d.Budget.FallbackBehavior == "" {
		d.Budget.FallbackBehavior = "track-tokens"
	}

	// ── Health / stuck detection config ────────────────────────────────────
	if v, ok := cm.Data["healthEnabled"]; ok {
		d.Health.Enabled = v == "true"
	}
	if v, ok := cm.Data["healthCheckIntervalSeconds"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.Health.CheckIntervalSeconds = n
		}
	}
	if d.Health.CheckIntervalSeconds == 0 {
		d.Health.CheckIntervalSeconds = 30
	}
	if v, ok := cm.Data["stuckDetectionEnabled"]; ok {
		d.Health.StuckDetection.Enabled = v == "true"
	}
	if v, ok := cm.Data["stuckDetectionToolDiversityMin"]; ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			d.Health.StuckDetection.ToolDiversityMin = f
		}
	}
	if d.Health.StuckDetection.ToolDiversityMin == 0 {
		d.Health.StuckDetection.ToolDiversityMin = 0.3
	}
	if v, ok := cm.Data["stuckDetectionMaxRepeatedPrompts"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.Health.StuckDetection.MaxRepeatedPrompts = n
		}
	}
	if d.Health.StuckDetection.MaxRepeatedPrompts == 0 {
		d.Health.StuckDetection.MaxRepeatedPrompts = 3
	}
	if v, ok := cm.Data["stuckDetectionStatusStaleMinutes"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.Health.StuckDetection.StatusStaleMinutes = n
		}
	}
	if d.Health.StuckDetection.StatusStaleMinutes == 0 {
		d.Health.StuckDetection.StatusStaleMinutes = 5
	}
	if v, ok := cm.Data["stuckDetectionCheckWindowMinutes"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.Health.StuckDetection.CheckWindowMinutes = n
		}
	}
	if d.Health.StuckDetection.CheckWindowMinutes == 0 {
		d.Health.StuckDetection.CheckWindowMinutes = 5
	}
	if v, ok := cm.Data["stuckDetectionAction"]; ok && v != "" {
		d.Health.StuckDetection.Action = v
	}
	if d.Health.StuckDetection.Action == "" {
		d.Health.StuckDetection.Action = "warn"
	}

	// ── Storage retained / knowledge discovery ────────────────────────────
	if v, ok := cm.Data["storageRetainedDiscovery"]; ok && v != "" {
		d.StorageRetained.Discovery = v
	}
	if d.StorageRetained.Discovery == "" {
		d.StorageRetained.Discovery = "tags"
	}
	if v, ok := cm.Data["storageRetainedAutoMount"]; ok {
		d.StorageRetained.AutoMount = v == "true"
	}
	if v, ok := cm.Data["storageRetainedMountMode"]; ok && v != "" {
		d.StorageRetained.MountMode = v
	}
	if d.StorageRetained.MountMode == "" {
		d.StorageRetained.MountMode = "readOnly"
	}
	if v, ok := cm.Data["storageRetainedStaleAfterDays"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.StorageRetained.StaleAfterDays = n
		}
	}
	if d.StorageRetained.StaleAfterDays == 0 {
		d.StorageRetained.StaleAfterDays = 90
	}
	if v, ok := cm.Data["storageRetainedMaxPerNamespace"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			d.StorageRetained.MaxRetainedPerNS = n
		}
	}
	if d.StorageRetained.MaxRetainedPerNS == 0 {
		d.StorageRetained.MaxRetainedPerNS = 20
	}

	// ── Cleanup TTL config ────────────────────────────────────────────────
	if v, ok := cm.Data["cleanupTTLCompleted"]; ok && v != "" {
		d.CleanupTTL.Completed = v
	}
	if d.CleanupTTL.Completed == "" {
		d.CleanupTTL.Completed = "7d"
	}
	if v, ok := cm.Data["cleanupTTLFailed"]; ok && v != "" {
		d.CleanupTTL.Failed = v
	}
	if d.CleanupTTL.Failed == "" {
		d.CleanupTTL.Failed = "2d"
	}
	if v, ok := cm.Data["cleanupTTLCancelled"]; ok && v != "" {
		d.CleanupTTL.Cancelled = v
	}
	if d.CleanupTTL.Cancelled == "" {
		d.CleanupTTL.Cancelled = "1d"
	}

	r.defaultsMu.Lock()
	r.defaults = d
	r.defaultsAt = time.Now()
	r.defaultsMu.Unlock()
}

// parseDurationString parses a duration string like "7d", "2d", "24h", "48h".
func parseDurationString(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	re := regexp.MustCompile(`^(\d+)d$`)
	m := re.FindStringSubmatch(s)
	if len(m) == 2 {
		days, _ := strconv.Atoi(m[1])
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("invalid duration: %s", s)
}

// setCompletionStatus sets CompletedAt and Duration on the task status.
func setCompletionStatus(task *corev1alpha1.AgentTask) {
	now := metav1.Now()
	task.Status.CompletedAt = &now
	if task.Status.StartedAt != nil {
		duration := now.Sub(task.Status.StartedAt.Time)
		task.Status.Duration = duration.Round(time.Second).String()
	}
}

// collectPodLogs retrieves the last maxOutputLen characters from the pod's agent container logs.
func (r *AgentTaskReconciler) collectPodLogs(ctx context.Context, namespace, podName string) string {
	if r.Clientset == nil {
		return ""
	}

	tailLines := int64(100)
	req := r.Clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: "agent",
		TailLines: &tailLines,
	})

	stream, err := req.Stream(ctx)
	if err != nil {
		log.FromContext(ctx).V(1).Info("Failed to get pod logs", "error", err)
		return ""
	}
	defer func() { _ = stream.Close() }()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, stream)
	if err != nil {
		return ""
	}

	output := buf.String()
	if len(output) > maxOutputLen {
		output = output[len(output)-maxOutputLen:]
	}
	return output
}

// notifyParentTask appends this task's name to the parent's status.childTasks list.
// For parents in the Waiting phase, it also injects the child's result into the
// parent's PVC at /inbox/child-results/<child-name>.json, and wakes up the parent
// when all pending children are terminal.
func (r *AgentTaskReconciler) notifyParentTask(ctx context.Context, task *corev1alpha1.AgentTask) {
	if task.Spec.ParentTaskID == "" {
		return
	}

	logger := log.FromContext(ctx)

	parent := &corev1alpha1.AgentTask{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: task.Namespace,
		Name:      task.Spec.ParentTaskID,
	}, parent); err != nil {
		logger.V(1).Info("Failed to fetch parent task", "parent", task.Spec.ParentTaskID, "error", err)
		return
	}

	// Append to childTasks if not already present
	found := false
	for _, child := range parent.Status.ChildTasks {
		if child == task.Name {
			found = true
			break
		}
	}
	if !found {
		parent.Status.ChildTasks = append(parent.Status.ChildTasks, task.Name)
	}

	// Inject child result into parent PVC for agentic tiers.
	// The result is written to /inbox/child-results/<child-name>.json inside
	// the parent's PVC so the reincarnated parent can read it.
	if isAgenticTier(parent.Spec.Tier) && isTerminalPhase(task.Status.Phase) {
		r.injectChildResult(ctx, parent, task)
	}

	// Remove from pending children
	remaining := make([]string, 0, len(parent.Status.PendingChildren))
	for _, pc := range parent.Status.PendingChildren {
		if pc != task.Name {
			remaining = append(remaining, pc)
		}
	}
	parent.Status.PendingChildren = remaining

	// If parent is Waiting and all pending children are done, wake up the parent
	if parent.Status.Phase == corev1alpha1.AgentTaskPhaseWaiting && len(remaining) == 0 {
		logger.Info("All children done, waking up parent", "parent", parent.Name)
		parent.Status.Phase = corev1alpha1.AgentTaskPhasePending
		parent.Status.Message = "Children completed, restarting agent"
		r.Recorder.Event(parent, corev1.EventTypeNormal, "TaskReincarnating",
			fmt.Sprintf("Child %s completed, all children done — restarting", task.Name))
	}

	if err := r.updateStatusWithRetry(ctx, parent); err != nil {
		logger.V(1).Info("Failed to update parent status", "error", err)
	}
}

// injectChildResult writes the child's output into the parent's PVC at
// /inbox/child-results/<child-name>.json using a short-lived exec into
// a utility pod. If exec isn't available, it falls back to storing the
// result in an annotation (for small results).
func (r *AgentTaskReconciler) injectChildResult(ctx context.Context,
	parent *corev1alpha1.AgentTask, child *corev1alpha1.AgentTask) {

	logger := log.FromContext(ctx)

	if r.Clientset == nil {
		logger.V(1).Info("No clientset, skipping child result injection")
		return
	}

	// Build the child result payload
	childResult := map[string]string{
		"taskId":  child.Name,
		"status":  string(child.Status.Phase),
		"output":  child.Status.Output,
		"message": child.Status.Message,
	}
	resultJSON, err := json.Marshal(childResult)
	if err != nil {
		logger.V(1).Info("Failed to marshal child result", "error", err)
		return
	}

	// Create a one-shot pod to write the result into the parent PVC.
	// This is necessary because the parent pod is not running (Waiting phase).
	pvcName := fmt.Sprintf("%s-storage", parent.Name)
	writerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-inject-%s", parent.Name, child.Name),
			Namespace: parent.Namespace,
			Labels: map[string]string{
				"hortator.ai/task":   parent.Name,
				"hortator.ai/inject": "child-result",
				"hortator.ai/source": child.Name,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "writer",
					Image: "busybox:1.37.0",
					Command: []string{"sh", "-c",
						`mkdir -p /inbox/child-results && printf '%s' "$RESULT_JSON" > /inbox/child-results/$CHILD_NAME.json`},
					Env: []corev1.EnvVar{
						{Name: "RESULT_JSON", Value: string(resultJSON)},
						{Name: "CHILD_NAME", Value: child.Name},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "parent-storage", MountPath: "/inbox", SubPath: "inbox"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "parent-storage",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}

	// Clean up any previous injection pod with the same name
	existing := &corev1.Pod{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: parent.Namespace,
		Name:      writerPod.Name,
	}, existing); err == nil {
		_ = r.Delete(ctx, existing)
	}

	if err := r.Create(ctx, writerPod); err != nil {
		logger.V(1).Info("Failed to create child result injection pod",
			"parent", parent.Name, "child", child.Name, "error", err)
	} else {
		logger.Info("Injecting child result into parent PVC",
			"parent", parent.Name, "child", child.Name)
	}
}

// extractTokenUsage parses agent logs to extract token usage from the runtime output.
func (r *AgentTaskReconciler) extractTokenUsage(task *corev1alpha1.AgentTask) {
	if task.Status.Output == "" {
		return
	}
	re := regexp.MustCompile(`Tokens: in=(\d+) out=(\d+)`)
	matches := re.FindStringSubmatch(task.Status.Output)
	if len(matches) == 3 {
		input, _ := strconv.ParseInt(matches[1], 10, 64)
		output, _ := strconv.ParseInt(matches[2], 10, 64)
		task.Status.TokensUsed = &corev1alpha1.TokenUsage{
			Input:  input,
			Output: output,
		}
	}
}

// extractResult pulls the actual LLM response from between result markers in logs.
func (r *AgentTaskReconciler) extractResult(task *corev1alpha1.AgentTask) {
	if task.Status.Output == "" {
		return
	}
	const beginMarker = "[hortator-result-begin]\n"
	const endMarker = "\n[hortator-result-end]"

	beginIdx := strings.Index(task.Status.Output, beginMarker)
	endIdx := strings.Index(task.Status.Output, endMarker)
	if beginIdx >= 0 && endIdx > beginIdx {
		result := task.Status.Output[beginIdx+len(beginMarker) : endIdx]
		task.Status.Output = strings.TrimSpace(result)
	}
}
