package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

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

	if err != nil {
		r.defaultsMu.Lock()
		r.defaults = ClusterDefaults{
			DefaultTimeout:        600,
			DefaultImage:          defaultImage,
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
func (r *AgentTaskReconciler) notifyParentTask(ctx context.Context, task *corev1alpha1.AgentTask) {
	if task.Spec.ParentTaskID == "" {
		return
	}

	parent := &corev1alpha1.AgentTask{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: task.Namespace,
		Name:      task.Spec.ParentTaskID,
	}, parent); err != nil {
		log.FromContext(ctx).V(1).Info("Failed to fetch parent task", "parent", task.Spec.ParentTaskID, "error", err)
		return
	}

	for _, child := range parent.Status.ChildTasks {
		if child == task.Name {
			return
		}
	}

	parent.Status.ChildTasks = append(parent.Status.ChildTasks, task.Name)
	if err := r.Status().Update(ctx, parent); err != nil {
		log.FromContext(ctx).V(1).Info("Failed to update parent childTasks", "error", err)
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
