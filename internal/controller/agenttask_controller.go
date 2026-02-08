/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

const (
	finalizerName = "agenttask.core.hortator.ai/finalizer"
	maxOutputLen  = 1000
)

// Prometheus metrics
var (
	tasksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hortator_tasks_total",
			Help: "Total number of AgentTasks by phase and namespace",
		},
		[]string{"phase", "namespace"},
	)
	tasksActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hortator_tasks_active",
			Help: "Number of currently active (Running) AgentTasks by namespace",
		},
		[]string{"namespace"},
	)
	taskDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hortator_task_duration_seconds",
			Help:    "Duration of completed AgentTasks in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1s to ~16384s
		},
	)
)

var tracer = otel.Tracer("hortator.ai/operator")

func init() {
	metrics.Registry.MustRegister(tasksTotal, tasksActive, taskDuration)
}

// taskEventAttrs returns common OTel attributes for a task event.
func taskEventAttrs(task *corev1alpha1.AgentTask) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("task.name", task.Name),
		attribute.String("task.namespace", task.Namespace),
		attribute.String("task.tier", task.Spec.Tier),
		attribute.String("task.role", task.Spec.Role),
		attribute.String("task.parentTaskId", task.Spec.ParentTaskID),
	}
	return attrs
}

// emitTaskEvent starts a span and records a named event with task attributes.
func emitTaskEvent(ctx context.Context, eventName string, task *corev1alpha1.AgentTask) {
	_, span := tracer.Start(ctx, eventName)
	defer span.End()
	span.AddEvent(eventName, trace.WithAttributes(taskEventAttrs(task)...))
}

// ClusterDefaults holds defaults read from the hortator-config ConfigMap.
type ClusterDefaults struct {
	DefaultTimeout         int
	DefaultImage           string
	DefaultRequestsCPU     string
	DefaultRequestsMemory  string
	DefaultLimitsCPU       string
	DefaultLimitsMemory    string
	EnforceNamespaceLabels bool
	PresidioEnabled        bool
	PresidioImage          string
	PresidioScoreThreshold string
	PresidioAction         string
}

// AgentTaskReconciler reconciles a AgentTask object
type AgentTaskReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Clientset kubernetes.Interface

	// Namespace the operator runs in (for ConfigMap lookup)
	Namespace string

	// Cached cluster defaults
	defaults ClusterDefaults
}

// +kubebuilder:rbac:groups=core.hortator.ai,resources=agentpolicies,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.hortator.ai,resources=agenttasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.hortator.ai,resources=agenttasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.hortator.ai,resources=agenttasks/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get

// Reconcile is the main reconciliation loop for AgentTask resources
func (r *AgentTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Refresh cluster defaults from ConfigMap (best-effort)
	r.loadClusterDefaults(ctx)

	// Fetch the AgentTask instance
	task := &corev1alpha1.AgentTask{}
	if err := r.Get(ctx, req.NamespacedName, task); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !task.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, task)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(task, finalizerName) {
		controllerutil.AddFinalizer(task, finalizerName)
		// Set retention annotation from spec if provided
		if task.Spec.Storage != nil && task.Spec.Storage.RetainDays != nil {
			if task.Annotations == nil {
				task.Annotations = map[string]string{}
			}
			task.Annotations["hortator.ai/retention"] = fmt.Sprintf("%dd", *task.Spec.Storage.RetainDays)
		}
		if err := r.Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize status if needed
	if task.Status.Phase == "" {
		task.Status.Phase = corev1alpha1.AgentTaskPhasePending
		task.Status.Message = "Task pending"
		emitTaskEvent(ctx, "hortator.task.created", task)
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Handle based on current phase
	switch task.Status.Phase {
	case corev1alpha1.AgentTaskPhasePending:
		return r.handlePending(ctx, task)
	case corev1alpha1.AgentTaskPhaseRunning:
		return r.handleRunning(ctx, task)
	case corev1alpha1.AgentTaskPhaseCancelled:
		if task.Annotations == nil || task.Annotations["hortator.ai/cancel-event-sent"] != "true" {
			emitTaskEvent(ctx, "hortator.task.cancelled", task)
			if task.Annotations == nil {
				task.Annotations = map[string]string{}
			}
			task.Annotations["hortator.ai/cancel-event-sent"] = "true"
			if err := r.Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
		}
		return r.handleTTLCleanup(ctx, task)
	case corev1alpha1.AgentTaskPhaseCompleted, corev1alpha1.AgentTaskPhaseFailed,
		corev1alpha1.AgentTaskPhaseTimedOut, corev1alpha1.AgentTaskPhaseBudgetExceeded:
		// TTL cleanup for terminal tasks
		return r.handleTTLCleanup(ctx, task)
	default:
		logger.Info("Unknown phase", "phase", task.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// loadClusterDefaults reads the hortator-config ConfigMap and caches defaults.
func (r *AgentTaskReconciler) loadClusterDefaults(ctx context.Context) {
	ns := r.Namespace
	if ns == "" {
		ns = "hortator-system"
	}

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: "hortator-config"}, cm)
	if err != nil {
		// Fall back to hardcoded defaults
		r.defaults = ClusterDefaults{
			DefaultTimeout:        600,
			DefaultImage:          "ghcr.io/hortator-ai/agent:latest",
			DefaultRequestsCPU:    "100m",
			DefaultRequestsMemory: "128Mi",
			DefaultLimitsCPU:      "500m",
			DefaultLimitsMemory:   "512Mi",
		}
		return
	}

	d := ClusterDefaults{
		DefaultTimeout:        600,
		DefaultImage:          "ghcr.io/hortator-ai/agent:latest",
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
	if v, ok := cm.Data["presidioImage"]; ok && v != "" {
		d.PresidioImage = v
	}
	if v, ok := cm.Data["presidioScoreThreshold"]; ok && v != "" {
		d.PresidioScoreThreshold = v
	}
	if v, ok := cm.Data["presidioAction"]; ok && v != "" {
		d.PresidioAction = v
	}

	r.defaults = d
}

// parseDurationString parses a duration string like "7d", "2d", "24h", "48h".
func parseDurationString(s string) (time.Duration, error) {
	// Try standard Go duration first
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	// Try Nd format (days)
	re := regexp.MustCompile(`^(\d+)d$`)
	m := re.FindStringSubmatch(s)
	if m != nil {
		days, _ := strconv.Atoi(m[1])
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("invalid duration: %s", s)
}

// handleTTLCleanup checks if a completed/failed task should be deleted based on retention.
func (r *AgentTaskReconciler) handleTTLCleanup(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	// Tasks with retain=true are exempt
	if task.Spec.Storage != nil && task.Spec.Storage.Retain {
		return ctrl.Result{}, nil
	}

	if task.Status.CompletedAt == nil {
		return ctrl.Result{}, nil
	}

	// Determine retention period
	defaultRetention := "7d"
	if task.Status.Phase == corev1alpha1.AgentTaskPhaseFailed {
		defaultRetention = "2d"
	}

	retention := defaultRetention
	if ann, ok := task.Annotations["hortator.ai/retention"]; ok && ann != "" {
		retention = ann
	}

	retentionDuration, err := parseDurationString(retention)
	if err != nil {
		log.FromContext(ctx).Error(err, "Invalid retention annotation, using default", "retention", retention)
		retentionDuration, _ = parseDurationString(defaultRetention)
	}

	elapsed := time.Since(task.Status.CompletedAt.Time)
	if elapsed < retentionDuration {
		// Requeue after remaining time
		remaining := retentionDuration - elapsed
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// TTL expired — delete the task (finalizer will clean up pod)
	log.FromContext(ctx).Info("TTL expired, deleting task", "task", task.Name, "retention", retention)
	if err := r.Delete(ctx, task); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when task is deleted
func (r *AgentTaskReconciler) handleDeletion(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(task, finalizerName) {
		emitTaskEvent(ctx, "hortator.task.deleted", task)
		// Cleanup: delete the pod if it exists
		if task.Status.PodName != "" {
			pod := &corev1.Pod{}
			err := r.Get(ctx, types.NamespacedName{
				Namespace: task.Namespace,
				Name:      task.Status.PodName,
			}, pod)
			if err == nil {
				logger.Info("Deleting pod", "pod", task.Status.PodName)
				if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
			}
		}

		// Decrement active gauge if it was running
		if task.Status.Phase == corev1alpha1.AgentTaskPhaseRunning {
			tasksActive.WithLabelValues(task.Namespace).Dec()
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(task, finalizerName)
		if err := r.Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// setCompletionStatus sets CompletedAt and Duration on the task status.
func setCompletionStatus(task *corev1alpha1.AgentTask) {
	now := metav1.Now()
	task.Status.CompletedAt = &now
	if task.Status.StartedAt != nil {
		duration := now.Time.Sub(task.Status.StartedAt.Time)
		task.Status.Duration = duration.Round(time.Second).String()
	}
}

// handlePending creates the pod for a pending task
func (r *AgentTaskReconciler) handlePending(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Namespace restriction: check if namespace has hortator.ai/enabled=true label
	if r.defaults.EnforceNamespaceLabels {
		ns := &corev1.Namespace{}
		if err := r.Get(ctx, types.NamespacedName{Name: task.Namespace}, ns); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to fetch namespace %s: %w", task.Namespace, err)
		}
		if ns.Labels["hortator.ai/enabled"] != "true" {
			task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
			task.Status.Message = "namespace not enabled for Hortator: add label hortator.ai/enabled=true"
			setCompletionStatus(task)
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
			if err := r.Status().Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Feature 6: Capability inheritance validation
	if task.Spec.ParentTaskID != "" {
		parent := &corev1alpha1.AgentTask{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: task.Namespace,
			Name:      task.Spec.ParentTaskID,
		}, parent); err != nil {
			if errors.IsNotFound(err) {
				task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
				task.Status.Message = fmt.Sprintf("parent task %s not found", task.Spec.ParentTaskID)
				setCompletionStatus(task)
				tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
				if err := r.Status().Update(ctx, task); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}

		// Check capabilities are a subset of parent's
		parentCaps := make(map[string]bool, len(parent.Spec.Capabilities))
		for _, c := range parent.Spec.Capabilities {
			parentCaps[c] = true
		}
		for _, cap := range task.Spec.Capabilities {
			if !parentCaps[cap] {
				task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
				task.Status.Message = fmt.Sprintf("capability escalation denied: %s not in parent capabilities", cap)
				setCompletionStatus(task)
				tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
				if err := r.Status().Update(ctx, task); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
		}
	}

	// Enforce AgentPolicy restrictions
	if violation := r.enforcePolicy(ctx, task); violation != "" {
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = fmt.Sprintf("policy violation: %s", violation)
		setCompletionStatus(task)
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Create PVC if needed (centurion/tribune tiers)
	if task.Spec.Tier == "centurion" || task.Spec.Tier == "tribune" {
		if err := r.ensurePVC(ctx, task); err != nil {
			logger.Error(err, "Failed to ensure PVC")
			task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
			task.Status.Message = fmt.Sprintf("Failed to create PVC: %v", err)
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
			if err := r.Status().Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Create the pod
	pod, err := r.buildPod(task)
	if err != nil {
		logger.Error(err, "Failed to build pod spec")
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = fmt.Sprintf("Failed to build pod: %v", err)
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(task, pod, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Create the pod
	if err := r.Create(ctx, pod); err != nil {
		if errors.IsAlreadyExists(err) {
			// Pod already exists, update status
			task.Status.Phase = corev1alpha1.AgentTaskPhaseRunning
			task.Status.PodName = pod.Name
			now := metav1.Now()
			task.Status.StartedAt = &now
			task.Status.Message = "Task running"
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseRunning), task.Namespace).Inc()
			tasksActive.WithLabelValues(task.Namespace).Inc()
			if err := r.Status().Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Created pod", "pod", pod.Name)
	emitTaskEvent(ctx, "hortator.task.started", task)

	// Update status
	task.Status.Phase = corev1alpha1.AgentTaskPhaseRunning
	task.Status.PodName = pod.Name
	now := metav1.Now()
	task.Status.StartedAt = &now
	task.Status.Message = "Task running"
	tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseRunning), task.Namespace).Inc()
	tasksActive.WithLabelValues(task.Namespace).Inc()
	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
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
	defer stream.Close()

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

	// Check if already tracked
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

// handleRunning monitors a running task
func (r *AgentTaskReconciler) handleRunning(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if task.Status.PodName == "" {
		// No pod name, something went wrong
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = "Pod name missing"
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		tasksActive.WithLabelValues(task.Namespace).Dec()
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Fetch the pod
	pod := &corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: task.Namespace,
		Name:      task.Status.PodName,
	}, pod); err != nil {
		if errors.IsNotFound(err) {
			// Pod was deleted externally
			task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
			task.Status.Message = "Pod was deleted"
			setCompletionStatus(task)
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
			tasksActive.WithLabelValues(task.Namespace).Dec()
			if err := r.Status().Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check pod status
	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		logger.Info("Pod succeeded", "pod", pod.Name)

		// Feature 3: Collect pod logs as output
		task.Status.Output = r.collectPodLogs(ctx, task.Namespace, task.Status.PodName)

		task.Status.Phase = corev1alpha1.AgentTaskPhaseCompleted
		task.Status.Message = "Task completed successfully"
		setCompletionStatus(task)
		emitTaskEvent(ctx, "hortator.task.completed", task)

		// Record metrics
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseCompleted), task.Namespace).Inc()
		tasksActive.WithLabelValues(task.Namespace).Dec()
		if task.Status.StartedAt != nil && task.Status.CompletedAt != nil {
			taskDuration.Observe(task.Status.CompletedAt.Time.Sub(task.Status.StartedAt.Time).Seconds())
		}

		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}

		// Feature 4: Notify parent task
		r.notifyParentTask(ctx, task)

		return ctrl.Result{}, nil

	case corev1.PodFailed:
		logger.Info("Pod failed", "pod", pod.Name)
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = "Task failed"
		setCompletionStatus(task)
		if len(pod.Status.ContainerStatuses) > 0 {
			cs := pod.Status.ContainerStatuses[0]
			if cs.State.Terminated != nil {
				task.Status.Message = fmt.Sprintf("Task failed: %s", cs.State.Terminated.Reason)
			}
		}

		// Collect logs even on failure
		task.Status.Output = r.collectPodLogs(ctx, task.Namespace, task.Status.PodName)
		emitTaskEvent(ctx, "hortator.task.failed", task)

		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		tasksActive.WithLabelValues(task.Namespace).Dec()
		if task.Status.StartedAt != nil && task.Status.CompletedAt != nil {
			taskDuration.Observe(task.Status.CompletedAt.Time.Sub(task.Status.StartedAt.Time).Seconds())
		}

		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}

		// Notify parent even on failure
		r.notifyParentTask(ctx, task)

		return ctrl.Result{}, nil

	case corev1.PodPending, corev1.PodRunning:
		// Check for timeout
		if task.Status.StartedAt != nil && task.Spec.Timeout != nil {
			timeout := time.Duration(*task.Spec.Timeout) * time.Second
			elapsed := time.Since(task.Status.StartedAt.Time)
			if elapsed > timeout {
				logger.Info("Task timed out", "elapsed", elapsed, "timeout", timeout)
				// Delete the pod
				if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				task.Status.Phase = corev1alpha1.AgentTaskPhaseTimedOut
				task.Status.Message = fmt.Sprintf("Task timed out after %s", timeout)
				setCompletionStatus(task)
				emitTaskEvent(ctx, "hortator.task.failed", task)
				tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseTimedOut), task.Namespace).Inc()
				tasksActive.WithLabelValues(task.Namespace).Dec()
				if err := r.Status().Update(ctx, task); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
		}
		// Continue monitoring
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil

	default:
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
}

// ensurePVC creates a PVC for centurion/tribune tiers if it doesn't already exist.
func (r *AgentTaskReconciler) ensurePVC(ctx context.Context, task *corev1alpha1.AgentTask) error {
	pvcName := fmt.Sprintf("%s-storage", task.Name)

	// Check if PVC already exists
	existing := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Namespace: task.Namespace, Name: pvcName}, existing)
	if err == nil {
		return nil // already exists
	}
	if !errors.IsNotFound(err) {
		return err
	}

	size := "1Gi"
	if task.Spec.Storage != nil && task.Spec.Storage.Size != "" {
		size = task.Spec.Storage.Size
	}

	annotations := map[string]string{}
	if task.Spec.Storage != nil && task.Spec.Storage.RetainDays != nil {
		annotations["hortator.ai/retention"] = fmt.Sprintf("%dd", *task.Spec.Storage.RetainDays)
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
					corev1.ResourceStorage: resource.MustParse(size),
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

// buildPod creates a pod spec for the agent task
func (r *AgentTaskReconciler) buildPod(task *corev1alpha1.AgentTask) (*corev1.Pod, error) {
	image := task.Spec.Image
	if image == "" {
		image = r.defaults.DefaultImage
		if image == "" {
			image = "ghcr.io/hortator-ai/agent:latest"
		}
	}

	// Build environment variables
	env := []corev1.EnvVar{
		{
			Name:  "HORTATOR_PROMPT",
			Value: task.Spec.Prompt,
		},
		{
			Name:  "HORTATOR_TASK_NAME",
			Value: task.Name,
		},
		{
			Name:  "HORTATOR_TASK_NAMESPACE",
			Value: task.Namespace,
		},
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

	// Add custom env vars (with SecretKeyRef support)
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

	// Build resource requirements (with cluster defaults fallback)
	resources := corev1.ResourceRequirements{}
	if task.Spec.Resources != nil {
		if task.Spec.Resources.Requests != nil {
			resources.Requests = corev1.ResourceList{}
			if task.Spec.Resources.Requests.CPU != "" {
				resources.Requests[corev1.ResourceCPU] = resource.MustParse(task.Spec.Resources.Requests.CPU)
			}
			if task.Spec.Resources.Requests.Memory != "" {
				resources.Requests[corev1.ResourceMemory] = resource.MustParse(task.Spec.Resources.Requests.Memory)
			}
		}
		if task.Spec.Resources.Limits != nil {
			resources.Limits = corev1.ResourceList{}
			if task.Spec.Resources.Limits.CPU != "" {
				resources.Limits[corev1.ResourceCPU] = resource.MustParse(task.Spec.Resources.Limits.CPU)
			}
			if task.Spec.Resources.Limits.Memory != "" {
				resources.Limits[corev1.ResourceMemory] = resource.MustParse(task.Spec.Resources.Limits.Memory)
			}
		}
	} else {
		// Apply cluster defaults
		resources.Requests = corev1.ResourceList{}
		resources.Limits = corev1.ResourceList{}
		if r.defaults.DefaultRequestsCPU != "" {
			resources.Requests[corev1.ResourceCPU] = resource.MustParse(r.defaults.DefaultRequestsCPU)
		}
		if r.defaults.DefaultRequestsMemory != "" {
			resources.Requests[corev1.ResourceMemory] = resource.MustParse(r.defaults.DefaultRequestsMemory)
		}
		if r.defaults.DefaultLimitsCPU != "" {
			resources.Limits[corev1.ResourceCPU] = resource.MustParse(r.defaults.DefaultLimitsCPU)
		}
		if r.defaults.DefaultLimitsMemory != "" {
			resources.Limits[corev1.ResourceMemory] = resource.MustParse(r.defaults.DefaultLimitsMemory)
		}
	}

	// Build volumes and volume mounts
	volumes, volumeMounts := r.buildVolumes(task)

	// Build init container to write task.json to /inbox
	taskSpecJSON, err := json.Marshal(task.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task spec: %w", err)
	}
	// Escape single quotes for shell
	escapedJSON := strings.ReplaceAll(string(taskSpecJSON), "'", "'\\''")

	initContainers := []corev1.Container{
		{
			Name:    "write-task-json",
			Image:   "busybox:latest",
			Command: []string{"sh", "-c", fmt.Sprintf("echo '%s' > /inbox/task.json", escapedJSON)},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "inbox", MountPath: "/inbox"},
			},
		},
	}

	// Presidio sidecar injection
	var presidioContainers []corev1.Container
	presidioEnabled := r.defaults.PresidioEnabled || task.Spec.Presidio != nil
	if presidioEnabled {
		presidioImage := r.defaults.PresidioImage
		if presidioImage == "" {
			presidioImage = "mcr.microsoft.com/presidio-analyzer:latest"
		}

		scoreThreshold := r.defaults.PresidioScoreThreshold
		if scoreThreshold == "" {
			scoreThreshold = "0.5"
		}
		presidioAction := r.defaults.PresidioAction
		if presidioAction == "" {
			presidioAction = "redact"
		}

		// Task-level overrides
		if task.Spec.Presidio != nil {
			if task.Spec.Presidio.ScoreThreshold != nil {
				scoreThreshold = fmt.Sprintf("%g", *task.Spec.Presidio.ScoreThreshold)
			}
			if task.Spec.Presidio.Action != "" {
				presidioAction = task.Spec.Presidio.Action
			}
		}

		sidecarEnv := []corev1.EnvVar{
			{Name: "PRESIDIO_SCORE_THRESHOLD", Value: scoreThreshold},
			{Name: "PRESIDIO_ACTION", Value: presidioAction},
		}

		sidecar := corev1.Container{
			Name:  "presidio",
			Image: presidioImage,
			Ports: []corev1.ContainerPort{{ContainerPort: 5001}},
			Env:   sidecarEnv,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/health",
						Port: intstr.FromInt(5001),
					},
				},
			},
		}

		// Mount custom config if configRef specified
		if task.Spec.Presidio != nil && task.Spec.Presidio.ConfigRef != "" {
			sidecar.VolumeMounts = append(sidecar.VolumeMounts, corev1.VolumeMount{
				Name:      "presidio-config",
				MountPath: "/etc/presidio/config.yaml",
				SubPath:   "config.yaml",
			})
			volumes = append(volumes, corev1.Volume{
				Name: "presidio-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: task.Spec.Presidio.ConfigRef,
						},
					},
				},
			})
		}

		presidioContainers = append(presidioContainers, sidecar)

		// Add PRESIDIO_ENDPOINT to agent container env
		env = append(env, corev1.EnvVar{
			Name:  "PRESIDIO_ENDPOINT",
			Value: "http://localhost:5001",
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
			RestartPolicy:  corev1.RestartPolicyNever,
			InitContainers: initContainers,
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

	// Append presidio sidecar containers
	pod.Spec.Containers = append(pod.Spec.Containers, presidioContainers...)

	return pod, nil
}

// buildVolumes returns volumes and volume mounts based on the task tier.
func (r *AgentTaskReconciler) buildVolumes(task *corev1alpha1.AgentTask) ([]corev1.Volume, []corev1.VolumeMount) {
	tier := task.Spec.Tier
	if tier == "" {
		tier = "legionary"
	}

	usePVC := tier == "centurion" || tier == "tribune"
	pvcName := fmt.Sprintf("%s-storage", task.Name)

	var volumes []corev1.Volume
	var mounts []corev1.VolumeMount

	if usePVC {
		// /workspace and /memory on PVC, /inbox and /outbox on EmptyDir
		volumes = []corev1.Volume{
			{Name: "inbox", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			{Name: "outbox", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			{Name: "storage", VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
			}},
		}
		mounts = []corev1.VolumeMount{
			{Name: "inbox", MountPath: "/inbox"},
			{Name: "outbox", MountPath: "/outbox"},
			{Name: "storage", MountPath: "/workspace", SubPath: "workspace"},
			{Name: "storage", MountPath: "/memory", SubPath: "memory"},
		}
	} else {
		// All EmptyDir for legionary
		volumes = []corev1.Volume{
			{Name: "inbox", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			{Name: "outbox", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			{Name: "memory", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		}
		mounts = []corev1.VolumeMount{
			{Name: "inbox", MountPath: "/inbox"},
			{Name: "outbox", MountPath: "/outbox"},
			{Name: "workspace", MountPath: "/workspace"},
			{Name: "memory", MountPath: "/memory"},
		}
	}

	return volumes, mounts
}

// tierRank returns a numeric rank for a tier string.
func tierRank(tier string) int {
	switch tier {
	case "legionary":
		return 1
	case "centurion":
		return 2
	case "tribune":
		return 3
	default:
		return 0
	}
}

// enforcePolicy checks all AgentPolicy objects in the task's namespace.
// Returns an empty string if all policies pass, or a violation description.
func (r *AgentTaskReconciler) enforcePolicy(ctx context.Context, task *corev1alpha1.AgentTask) string {
	policies := &corev1alpha1.AgentPolicyList{}
	if err := r.List(ctx, policies, client.InNamespace(task.Namespace)); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list AgentPolicies")
		return ""
	}

	if len(policies.Items) == 0 {
		return ""
	}

	for _, policy := range policies.Items {
		p := policy.Spec

		// Check denied capabilities first (overrides allowed)
		if len(p.DeniedCapabilities) > 0 {
			denied := make(map[string]bool, len(p.DeniedCapabilities))
			for _, c := range p.DeniedCapabilities {
				denied[c] = true
			}
			for _, cap := range task.Spec.Capabilities {
				if denied[cap] {
					return fmt.Sprintf("capability %q is denied by policy %s", cap, policy.Name)
				}
			}
		}

		// Check allowed capabilities
		if len(p.AllowedCapabilities) > 0 {
			allowed := make(map[string]bool, len(p.AllowedCapabilities))
			for _, c := range p.AllowedCapabilities {
				allowed[c] = true
			}
			for _, cap := range task.Spec.Capabilities {
				if !allowed[cap] {
					return fmt.Sprintf("capability %q is not allowed by policy %s", cap, policy.Name)
				}
			}
		}

		// Check allowed images
		if len(p.AllowedImages) > 0 {
			image := task.Spec.Image
			if image == "" {
				image = r.defaults.DefaultImage
			}
			matched := false
			for _, pattern := range p.AllowedImages {
				if ok, _ := path.Match(pattern, image); ok {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Sprintf("image %q is not allowed by policy %s", image, policy.Name)
			}
		}

		// Check max budget
		if p.MaxBudget != nil && task.Spec.Budget != nil {
			if p.MaxBudget.MaxTokens != nil && task.Spec.Budget.MaxTokens != nil {
				if *task.Spec.Budget.MaxTokens > *p.MaxBudget.MaxTokens {
					return fmt.Sprintf("token budget %d exceeds policy %s limit of %d", *task.Spec.Budget.MaxTokens, policy.Name, *p.MaxBudget.MaxTokens)
				}
			}
			if p.MaxBudget.MaxCostUsd != "" && task.Spec.Budget.MaxCostUsd != "" {
				policyVal, err1 := strconv.ParseFloat(p.MaxBudget.MaxCostUsd, 64)
				taskVal, err2 := strconv.ParseFloat(task.Spec.Budget.MaxCostUsd, 64)
				if err1 == nil && err2 == nil && taskVal > policyVal {
					return fmt.Sprintf("cost budget %s exceeds policy %s limit of %s", task.Spec.Budget.MaxCostUsd, policy.Name, p.MaxBudget.MaxCostUsd)
				}
			}
		}

		// Check max timeout
		if p.MaxTimeout != nil && task.Spec.Timeout != nil {
			if *task.Spec.Timeout > *p.MaxTimeout {
				return fmt.Sprintf("timeout %d exceeds policy %s limit of %d", *task.Spec.Timeout, policy.Name, *p.MaxTimeout)
			}
		}

		// Check max tier
		if p.MaxTier != "" {
			taskTier := task.Spec.Tier
			if taskTier == "" {
				taskTier = "legionary"
			}
			if tierRank(taskTier) > tierRank(p.MaxTier) {
				return fmt.Sprintf("tier %q exceeds policy %s max tier %q", taskTier, policy.Name, p.MaxTier)
			}
		}

		// Check max concurrent tasks
		if p.MaxConcurrentTasks != nil {
			taskList := &corev1alpha1.AgentTaskList{}
			if err := r.List(ctx, taskList, client.InNamespace(task.Namespace)); err == nil {
				running := 0
				for _, t := range taskList.Items {
					if t.Status.Phase == corev1alpha1.AgentTaskPhaseRunning {
						running++
					}
				}
				if running >= *p.MaxConcurrentTasks {
					return fmt.Sprintf("namespace has %d running tasks, policy %s limits to %d", running, policy.Name, *p.MaxConcurrentTasks)
				}
			}
		}
	}

	return ""
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.AgentTask{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
