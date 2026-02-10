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
	"math/rand"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
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
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

const (
	finalizerName         = "agenttask.core.hortator.ai/finalizer"
	maxOutputLen          = 16000
	defaultConfigCacheTTL = 30 * time.Second
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
	PresidioEndpoint       string
}

// AgentTaskReconciler reconciles a AgentTask object
type AgentTaskReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Clientset kubernetes.Interface

	// Namespace the operator runs in (for ConfigMap lookup)
	Namespace string

	// Cached cluster defaults with TTL to avoid K8s API calls on every reconcile.
	defaults    ClusterDefaults
	defaultsMu  sync.RWMutex
	defaultsAt  time.Time
	defaultsTTL time.Duration // 0 means use defaultConfigCacheTTL
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

	// Refresh cluster defaults from ConfigMap (best-effort, cached with TTL)
	r.refreshDefaultsIfStale(ctx)

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
	case corev1alpha1.AgentTaskPhaseRetrying:
		return r.handleRetrying(ctx, task)
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

// refreshDefaultsIfStale reloads cluster defaults only if the cache TTL has expired.
// This avoids a K8s API call to fetch the ConfigMap on every single reconciliation.
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
	// Read default image from env var (set by Helm deployment), fall back to hardcoded
	defaultImage := os.Getenv("HORTATOR_DEFAULT_AGENT_IMAGE")
	if defaultImage == "" {
		defaultImage = "ghcr.io/hortator-ai/agent:latest"
	}

	if err != nil {
		// Fall back to env/hardcoded defaults
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

	r.defaultsMu.Lock()
	r.defaults = d
	r.defaultsAt = time.Now()
	r.defaultsMu.Unlock()
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

	// Create PVC for every task — all tiers get persistent storage so agents
	// can produce artifacts (code, patches, reports) that survive pod completion.
	// Legionaries get smaller default PVCs, cleaned up by TTL.
	{
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

		// The agent reports results directly to the CRD via `hortator report`.
		// If the agent hasn't reported yet (legacy runtime or crash), fall back
		// to extracting from pod logs for backward compatibility.
		if task.Status.Output == "" {
			task.Status.Output = r.collectPodLogs(ctx, task.Namespace, task.Status.PodName)
			r.extractTokenUsage(task)
			r.extractResult(task)
		}
		r.recordAttempt(task, nil, "completed")

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

		// Determine failure reason and agent exit code
		failureReason := "Task failed"
		var agentExitCode *int32
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Name == "agent" && cs.State.Terminated != nil {
				agentExitCode = &cs.State.Terminated.ExitCode
				failureReason = fmt.Sprintf("Task failed: %s (exit %d)", cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
			}
		}

		// Check if agent actually succeeded (exit 0) but sidecar failed
		if agentExitCode != nil && *agentExitCode == 0 {
			logger.Info("Agent succeeded but sidecar failed, treating as success", "pod", pod.Name)
			task.Status.Output = r.collectPodLogs(ctx, task.Namespace, task.Status.PodName)
			r.extractTokenUsage(task)
			r.extractResult(task)
			task.Status.Phase = corev1alpha1.AgentTaskPhaseCompleted
			task.Status.Message = "Task completed (sidecar failed)"
			setCompletionStatus(task)
			r.recordAttempt(task, agentExitCode, "completed (sidecar failed)")
			emitTaskEvent(ctx, "hortator.task.completed", task)
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseCompleted), task.Namespace).Inc()
			tasksActive.WithLabelValues(task.Namespace).Dec()
			if err := r.Status().Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			r.notifyParentTask(ctx, task)
			return ctrl.Result{}, nil
		}

		// Collect logs before deciding on retry
		task.Status.Output = r.collectPodLogs(ctx, task.Namespace, task.Status.PodName)

		// Classify failure: transient (pod crash, no result.json) vs logical (agent reported failure)
		isTransient := r.isTransientFailure(ctx, task, pod)

		// Record this attempt
		r.recordAttempt(task, agentExitCode, failureReason)

		// Try retry if transient and retries configured
		if isTransient && r.shouldRetry(task) {
			backoff := r.computeBackoff(task)
			nextRetry := metav1.NewTime(time.Now().Add(backoff))
			task.Status.Phase = corev1alpha1.AgentTaskPhaseRetrying
			task.Status.NextRetryTime = &nextRetry
			task.Status.PodName = "" // Clear for next attempt
			task.Status.Message = fmt.Sprintf("Retrying in %s (attempt %d/%d): %s",
				backoff.Round(time.Second), task.Status.Attempts, task.Spec.Retry.MaxAttempts, failureReason)
			emitTaskEvent(ctx, "hortator.task.retrying", task)
			logger.Info("Scheduling retry", "attempt", task.Status.Attempts, "backoff", backoff)

			if err := r.Status().Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: backoff}, nil
		}

		// No retry — terminal failure
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = failureReason
		setCompletionStatus(task)
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

	// Default PVC size by tier: legionaries get less, centurion/tribune get more
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

	// Inject API key from model.apiKeyRef as ANTHROPIC_API_KEY / OPENAI_API_KEY
	if task.Spec.Model != nil && task.Spec.Model.ApiKeyRef != nil {
		// Determine env var name based on endpoint
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
	resources, err := r.buildResources(task)
	if err != nil {
		return nil, fmt.Errorf("invalid resource spec: %w", err)
	}

	// Build volumes and volume mounts
	volumes, volumeMounts := r.buildVolumes(task)

	// Build init container to write task.json to /inbox.
	// We pass the JSON via environment variable and use printf to avoid shell
	// interpolation vulnerabilities (backticks, $(), etc. in prompts).
	taskSpecJSON, err := json.Marshal(task.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task spec: %w", err)
	}

	initContainers := []corev1.Container{
		{
			Name:  "write-task-json",
			Image: "busybox:1.37.0",
			// Write from env var — no shell interpolation of user input.
			Command: []string{"sh", "-c", `printf '%s' "$TASK_JSON" > /inbox/task.json`},
			Env: []corev1.EnvVar{
				{Name: "TASK_JSON", Value: string(taskSpecJSON)},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "inbox", MountPath: "/inbox"},
			},
		},
	}

	// Presidio PII detection — centralized service (Deployment+Service in hortator-system)
	// Agent pods call the shared Presidio service via cluster DNS instead of a sidecar.
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
// Every task gets a PVC so agents can produce artifacts (code, patches, etc.)
// that survive pod completion. /inbox is EmptyDir (ephemeral input from operator).
func (r *AgentTaskReconciler) buildVolumes(task *corev1alpha1.AgentTask) ([]corev1.Volume, []corev1.VolumeMount) {
	pvcName := fmt.Sprintf("%s-storage", task.Name)

	volumes := []corev1.Volume{
		{Name: "inbox", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "storage", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName},
		}},
	}
	mounts := []corev1.VolumeMount{
		{Name: "inbox", MountPath: "/inbox"},
		{Name: "storage", MountPath: "/outbox", SubPath: "outbox"},
		{Name: "storage", MountPath: "/workspace", SubPath: "workspace"},
		{Name: "storage", MountPath: "/memory", SubPath: "memory"},
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

// parseQuantity parses a resource string, returning a clean error instead of panicking.
func parseQuantity(value, label string) (resource.Quantity, error) {
	qty, err := resource.ParseQuantity(value)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("invalid %s %q: %w", label, value, err)
	}
	return qty, nil
}

// buildResources constructs resource requirements from the task spec or cluster defaults.
// Uses ParseQuantity instead of MustParse to avoid panics on invalid input.
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
		// Apply cluster defaults
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

// extractTokenUsage parses agent logs to extract token usage from the runtime output.
// Looks for: "[hortator-runtime] Done. Tokens: in=N out=M"
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

// extractResult pulls the actual LLM response from between
// [hortator-result-begin] and [hortator-result-end] markers in the log output.
// If markers are found, status.output is replaced with just the result content.
// If not found (older runtime), status.output keeps the raw logs as before.
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

// handleRetrying checks if it's time to retry and transitions back to Pending.
func (r *AgentTaskReconciler) handleRetrying(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	if task.Status.NextRetryTime == nil {
		// Shouldn't happen, but recover
		task.Status.Phase = corev1alpha1.AgentTaskPhasePending
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	remaining := time.Until(task.Status.NextRetryTime.Time)
	if remaining > 0 {
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// Time to retry — transition back to Pending
	log.FromContext(ctx).Info("Retrying task", "task", task.Name, "attempt", task.Status.Attempts+1)
	task.Status.Phase = corev1alpha1.AgentTaskPhasePending
	task.Status.NextRetryTime = nil
	task.Status.Message = fmt.Sprintf("Retry attempt %d", task.Status.Attempts+1)
	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// isTransientFailure classifies a pod failure as transient or logical.
// Transient: pod crashed without writing result.json (exit != 0, no result)
// Logical: agent reported failure in result.json (deliberate)
func (r *AgentTaskReconciler) isTransientFailure(ctx context.Context, task *corev1alpha1.AgentTask, pod *corev1.Pod) bool {
	// If agent container exited with 0, it's not transient (it completed, possibly with a logical failure)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == "agent" && cs.State.Terminated != nil {
			if cs.State.Terminated.ExitCode == 0 {
				return false // Agent completed normally, any failure is logical
			}
		}
	}
	// Non-zero exit = transient (crash, OOM, etc.)
	return true
}

// shouldRetry returns true if the task has retries configured and hasn't exhausted them.
func (r *AgentTaskReconciler) shouldRetry(task *corev1alpha1.AgentTask) bool {
	if task.Spec.Retry == nil || task.Spec.Retry.MaxAttempts <= 0 {
		return false
	}
	return task.Status.Attempts < task.Spec.Retry.MaxAttempts
}

// computeBackoff returns the backoff duration for the current attempt.
func (r *AgentTaskReconciler) computeBackoff(task *corev1alpha1.AgentTask) time.Duration {
	base := 30
	max := 300
	if task.Spec.Retry != nil {
		if task.Spec.Retry.BackoffSeconds > 0 {
			base = task.Spec.Retry.BackoffSeconds
		}
		if task.Spec.Retry.MaxBackoffSeconds > 0 {
			max = task.Spec.Retry.MaxBackoffSeconds
		}
	}

	// Exponential backoff with jitter: base * 2^(attempts-1) ± 25%.
	// Jitter prevents thundering herd when many tasks fail simultaneously
	// (e.g., during an API outage) and all retry at the exact same time.
	backoff := base
	for i := 1; i < task.Status.Attempts; i++ {
		backoff *= 2
		if backoff > max {
			backoff = max
			break
		}
	}
	// Add ±25% jitter
	jitter := rand.Intn(backoff/2+1) - backoff/4
	backoff += jitter
	if backoff < 1 {
		backoff = 1
	}
	return time.Duration(backoff) * time.Second
}

// recordAttempt appends an attempt record to task status history.
func (r *AgentTaskReconciler) recordAttempt(task *corev1alpha1.AgentTask, exitCode *int32, reason string) {
	task.Status.Attempts++
	now := metav1.Now()
	record := corev1alpha1.AttemptRecord{
		Attempt: task.Status.Attempts,
		EndTime: &now,
		Reason:  reason,
	}
	if task.Status.StartedAt != nil {
		record.StartTime = *task.Status.StartedAt
	}
	if exitCode != nil {
		record.ExitCode = exitCode
	}
	task.Status.History = append(task.Status.History, record)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.AgentTask{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
