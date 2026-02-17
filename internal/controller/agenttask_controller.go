/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

const (
	finalizerName         = "agenttask.core.hortator.ai/finalizer"
	maxOutputLen          = 16000
	defaultConfigCacheTTL = 30 * time.Second
)

// ClusterDefaults holds defaults read from the hortator-config ConfigMap.
type ClusterDefaults struct {
	DefaultTimeout             int
	DefaultImage               string
	AgenticImage               string // Python agentic runtime for tribune/centurion tiers
	DefaultRequestsCPU         string
	DefaultRequestsMemory      string
	DefaultLimitsCPU           string
	DefaultLimitsMemory        string
	EnforceNamespaceLabels     bool
	PresidioEnabled            bool
	PresidioEndpoint           string
	PresidioAnonymizerEndpoint string
	PresidioScoreThreshold     string
	PresidioRequire            bool
	WarmPool                   WarmPoolConfig
	ResultCacheEnabled         bool
	ResultCacheTTL             time.Duration
	ResultCacheMaxEntries      int
	Budget                     BudgetConfig
	Health                     HealthConfig
	StorageRetained            StorageRetainedConfig
	CleanupTTL                 CleanupTTLConfig
	VectorStoreEnabled         bool
	VectorStoreProvider        string
	VectorStoreEndpoint        string
}

// BudgetConfig holds budget enforcement settings from the ConfigMap.
type BudgetConfig struct {
	Enabled           bool
	DefaultMaxCostUsd string
	WarningPercent    int
	SoftCeilingAction string // winddown | warn-only | hard-kill
	GraceMaxLLMCalls  int
	GraceMaxSeconds   int
	PriceSource       string // litellm | custom
	RefreshIntervalH  int
	FallbackBehavior  string // track-tokens | block | warn
}

// HealthConfig holds health/stuck-detection settings from the ConfigMap.
type HealthConfig struct {
	Enabled              bool
	CheckIntervalSeconds int
	StuckDetection       StuckDetectionConfig
}

// StuckDetectionConfig holds stuck detection thresholds.
type StuckDetectionConfig struct {
	Enabled            bool
	ToolDiversityMin   float64
	MaxRepeatedPrompts int
	StatusStaleMinutes int
	CheckWindowMinutes int
	Action             string // warn | kill | escalate
}

// StorageRetainedConfig holds retained PVC discovery settings.
type StorageRetainedConfig struct {
	Discovery        string // none | tags | semantic
	AutoMount        bool
	MountMode        string // readOnly
	StaleAfterDays   int
	MaxRetainedPerNS int
}

// CleanupTTLConfig holds CR garbage-collection TTLs.
type CleanupTTLConfig struct {
	Completed string // e.g. "7d", "24h"
	Failed    string // e.g. "2d", "48h"
	Cancelled string // e.g. "1d"
}

// AgentTaskReconciler reconciles a AgentTask object
type AgentTaskReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Clientset  kubernetes.Interface
	RESTConfig *rest.Config

	// Recorder emits K8s Events on AgentTask objects, visible via kubectl describe.
	Recorder record.EventRecorder

	// Namespace the operator runs in (for ConfigMap lookup)
	Namespace string

	// Cached cluster defaults with TTL to avoid K8s API calls on every reconcile.
	defaults    ClusterDefaults
	defaultsMu  sync.RWMutex
	defaultsAt  time.Time
	defaultsTTL time.Duration // 0 means use defaultConfigCacheTTL

	// Last warm pool reconciliation time (cooldown)
	warmPoolAt time.Time

	// Result cache for deduplication of identical prompt+role tasks.
	ResultCache *ResultCache

	// PriceMap caches LiteLLM model pricing for budget enforcement.
	PriceMap *PriceMap
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
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create

// Reconcile is the main reconciliation loop for AgentTask resources
func (r *AgentTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Refresh cluster defaults from ConfigMap (best-effort, cached with TTL)
	r.refreshDefaultsIfStale(ctx)

	// Lazy-init and refresh price map for budget enforcement
	r.defaultsMu.RLock()
	budgetEnabled := r.defaults.Budget.Enabled
	refreshH := r.defaults.Budget.RefreshIntervalH
	r.defaultsMu.RUnlock()
	if budgetEnabled {
		if r.PriceMap == nil {
			r.PriceMap = NewPriceMap(refreshH)
		}
		r.PriceMap.RefreshIfStale()
	}

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

	// Add finalizer if missing
	if !controllerutil.ContainsFinalizer(task, finalizerName) {
		controllerutil.AddFinalizer(task, finalizerName)
		if err := r.Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// If retain-pvc was set post-creation, strip owner reference from the PVC
	// so it isn't cascade-deleted when the AgentTask is removed.
	if task.Annotations != nil && task.Annotations["hortator.ai/retain-pvc"] == "true" {
		r.removeOwnerRefFromPVC(ctx, task)
	}

	logger.V(1).Info("Reconciling task", "task", task.Name, "phase", task.Status.Phase)

	// Periodically reconcile warm pool (piggyback on task reconciliation)
	if r.defaults.WarmPool.Enabled {
		if err := r.reconcileWarmPool(ctx); err != nil {
			logger.Error(err, "Failed to reconcile warm pool")
		}
	}

	// Phase machine
	switch task.Status.Phase {
	case "", corev1alpha1.AgentTaskPhasePending:
		return r.handlePending(ctx, task)
	case corev1alpha1.AgentTaskPhaseRunning:
		return r.handleRunning(ctx, task)
	case corev1alpha1.AgentTaskPhaseWaiting:
		return r.handleWaiting(ctx, task)
	case corev1alpha1.AgentTaskPhaseRetrying:
		return r.handleRetrying(ctx, task)
	case corev1alpha1.AgentTaskPhaseCompleted, corev1alpha1.AgentTaskPhaseFailed,
		corev1alpha1.AgentTaskPhaseTimedOut, corev1alpha1.AgentTaskPhaseBudgetExceeded,
		corev1alpha1.AgentTaskPhaseCancelled:
		// Accumulate usage to hierarchy budget on first terminal reconcile
		if task.Annotations == nil {
			task.Annotations = map[string]string{}
		}
		if _, done := task.Annotations["hortator.ai/hierarchy-accounted"]; !done {
			r.updateHierarchyBudget(ctx, task)
			task.Annotations["hortator.ai/hierarchy-accounted"] = "true"
			if err := r.Update(ctx, task); err != nil {
				logger.V(1).Info("Failed to mark hierarchy-accounted", "error", err)
			}
		}
		return r.handleTTLCleanup(ctx, task)
	default:
		logger.Info("Unknown phase", "phase", task.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create

// handleTTLCleanup deletes terminal tasks (and their PVCs) after their retention
// period expires. Respects the hortator.ai/retain annotation and per-phase TTL
// configuration from the ConfigMap (CleanupTTL).
func (r *AgentTaskReconciler) handleTTLCleanup(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	// Skip if explicitly retained via annotation (keeps both task CR and PVC).
	// Note: storage.retain only preserves the PVC, not the task CR itself.
	if ann, ok := task.Annotations["hortator.ai/retain"]; ok && ann == "true" {
		return ctrl.Result{}, nil
	}

	if task.Status.CompletedAt == nil {
		return ctrl.Result{}, nil
	}

	// Use configurable TTL from CleanupTTL config, falling back to sensible defaults.
	r.defaultsMu.RLock()
	ttlCfg := r.defaults.CleanupTTL
	r.defaultsMu.RUnlock()

	var defaultRetention string
	switch task.Status.Phase {
	case corev1alpha1.AgentTaskPhaseFailed:
		defaultRetention = ttlCfg.Failed
	case corev1alpha1.AgentTaskPhaseCancelled:
		defaultRetention = ttlCfg.Cancelled
	default:
		defaultRetention = ttlCfg.Completed
	}
	if defaultRetention == "" {
		defaultRetention = "1h"
	}

	retention := defaultRetention
	if ann, ok := task.Annotations["hortator.ai/retention"]; ok && ann != "" && ann != "true" {
		retention = ann
	}

	retentionDuration, err := parseDurationString(retention)
	if err != nil {
		log.FromContext(ctx).Error(err, "Invalid retention annotation, using default", "retention", retention)
		retentionDuration, _ = parseDurationString(defaultRetention)
	}

	elapsed := time.Since(task.Status.CompletedAt.Time)
	if elapsed < retentionDuration {
		remaining := retentionDuration - elapsed
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	logger := log.FromContext(ctx)
	retainPVC := task.Spec.Storage != nil && task.Spec.Storage.Retain
	logger.Info("TTL expired, cleaning up task", "task", task.Name, "retention", retention, "retainPVC", retainPVC)

	// Delete the associated PVC if it exists (unless storage.retain is set)
	if !retainPVC {
		pvcName := fmt.Sprintf("%s-storage", task.Name)
		pvc := &corev1.PersistentVolumeClaim{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: task.Namespace, Name: pvcName}, pvc); err == nil {
			if delErr := r.Delete(ctx, pvc); delErr != nil && !errors.IsNotFound(delErr) {
				logger.Error(delErr, "Failed to delete PVC during GC", "pvc", pvcName)
			} else {
				logger.V(1).Info("Deleted PVC during GC", "pvc", pvcName)
			}
		}
	}

	emitTaskEvent(ctx, "hortator.task.garbage_collected", task)

	// Delete the AgentTask CR itself
	if err := r.Delete(ctx, task); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when task is deleted.
func (r *AgentTaskReconciler) handleDeletion(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(task, finalizerName) {
		emitTaskEvent(ctx, "hortator.task.deleted", task)
		if task.Status.PodName != "" {
			pod := &corev1.Pod{}
			err := r.Get(ctx, client.ObjectKey{
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

		if task.Status.Phase == corev1alpha1.AgentTaskPhaseRunning {
			tasksActive.WithLabelValues(task.Namespace).Dec()
		}

		controllerutil.RemoveFinalizer(task, finalizerName)
		if err := r.Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// handlePending creates the pod for a pending task.
func (r *AgentTaskReconciler) handlePending(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Namespace restriction
	if r.defaults.EnforceNamespaceLabels {
		ns := &corev1.Namespace{}
		if err := r.Get(ctx, client.ObjectKey{Name: task.Namespace}, ns); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to fetch namespace %s: %w", task.Namespace, err)
		}
		if ns.Labels["hortator.ai/enabled"] != "true" {
			task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
			task.Status.Message = "namespace not enabled for Hortator: add label hortator.ai/enabled=true"
			setCompletionStatus(task)
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
			if err := r.updateStatusWithRetry(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// Hierarchy budget check — reject new children if tree budget exhausted
	if reason := r.checkHierarchyBudgetExhausted(ctx, task); reason != "" {
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = fmt.Sprintf("Hierarchy budget exhausted: %s", reason)
		setCompletionStatus(task)
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Capability inheritance validation
	if task.Spec.ParentTaskID != "" {
		parent := &corev1alpha1.AgentTask{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: task.Namespace,
			Name:      task.Spec.ParentTaskID,
		}, parent); err != nil {
			if errors.IsNotFound(err) {
				task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
				task.Status.Message = fmt.Sprintf("parent task %s not found", task.Spec.ParentTaskID)
				setCompletionStatus(task)
				tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
				if err := r.updateStatusWithRetry(ctx, task); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}

		// Register this child in the parent's PendingChildren if not already tracked.
		// This is critical for reincarnation: when a reincarnated parent spawns new
		// children, the operator must track them so the Waiting → Pending transition
		// fires after all children complete.
		if !containsString(parent.Status.PendingChildren, task.Name) {
			parent.Status.PendingChildren = append(parent.Status.PendingChildren, task.Name)
			if err := r.updateStatusWithRetry(ctx, parent); err != nil {
				logger.V(1).Info("Failed to register child in parent PendingChildren",
					"child", task.Name, "parent", parent.Name, "error", err)
			} else {
				logger.Info("Registered child in parent PendingChildren",
					"child", task.Name, "parent", parent.Name,
					"pendingCount", len(parent.Status.PendingChildren))
			}
		}

		// Inherit model spec from parent if child doesn't specify one
		if task.Spec.Model == nil && parent.Spec.Model != nil {
			task.Spec.Model = parent.Spec.Model.DeepCopy()
			logger.Info("Inherited model spec from parent", "task", task.Name, "parent", parent.Name, "model", parent.Spec.Model.Name)
			// Persist the inherited model to the CRD spec
			if err := r.Update(ctx, task); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to persist inherited model: %w", err)
			}
		}

		// Build effective parent capabilities including auto-injected ones
		// (e.g., "spawn" is auto-injected for tribune/centurion tiers).
		parentEffectiveCaps := effectiveCapabilities(parent.Spec.Tier, parent.Spec.Capabilities)
		parentCaps := make(map[string]bool, len(parentEffectiveCaps))
		for _, c := range parentEffectiveCaps {
			parentCaps[c] = true
		}
		for _, cap := range task.Spec.Capabilities {
			if !parentCaps[cap] {
				msg := fmt.Sprintf("capability escalation denied: child requested [%s] but parent only has %v",
					cap, parentEffectiveCaps)
				task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
				task.Status.Message = msg
				setCompletionStatus(task)
				tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
				r.Recorder.Eventf(task, corev1.EventTypeWarning, "CapabilityEscalation", msg)
				if err := r.updateStatusWithRetry(ctx, task); err != nil {
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
		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Check hierarchy budget (shared budget across task tree)
	if reason := r.checkHierarchyBudgetExhausted(ctx, task); reason != "" {
		task.Status.Phase = corev1alpha1.AgentTaskPhaseCancelled
		task.Status.Message = reason
		setCompletionStatus(task)
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseCancelled), task.Namespace).Inc()
		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Check result cache before spawning (if enabled and task hasn't opted out).
	// Cache hits return immediately without creating any K8s resources — the task
	// transitions directly to Completed with the cached output.
	if r.ResultCache != nil && !shouldSkipCache(task) {
		modelName := ""
		if task.Spec.Model != nil {
			modelName = task.Spec.Model.Name
		}
		cacheKey := CacheKey(task.Spec.Prompt, task.Spec.Role, modelName, task.Spec.Tier)
		if cached := r.ResultCache.Get(cacheKey); cached != nil {
			logger.Info("Cache hit", "task", task.Name, "key", cacheKey[:12])
			task.Status.Phase = corev1alpha1.AgentTaskPhaseCompleted
			task.Status.Output = cached.Output
			task.Status.Message = "Completed (cache hit)"
			task.Status.TokensUsed = &corev1alpha1.TokenUsage{
				Input:  cached.TokensIn,
				Output: cached.TokensOut,
			}
			now := metav1.Now()
			task.Status.StartedAt = &now
			setCompletionStatus(task)
			// Annotate for observability
			if task.Annotations == nil {
				task.Annotations = make(map[string]string)
			}
			task.Annotations["hortator.ai/cache-hit"] = cacheKey[:12]
			if err := r.Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseCompleted), task.Namespace).Inc()
			emitTaskEvent(ctx, "hortator.task.completed.cached", task)
			r.Recorder.Event(task, corev1.EventTypeNormal, "CacheHit", "Result served from cache")
			if err := r.updateStatusWithRetry(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			r.notifyParentTask(ctx, task)
			return ctrl.Result{}, nil
		}
	}

	// Try warm pool first (if enabled)
	if r.defaults.WarmPool.Enabled {
		if pod, err := r.claimWarmPod(ctx, task); err != nil {
			logger.Error(err, "Failed to claim warm pod, falling back to normal creation")
		} else if pod != nil {
			// Inject task into warm pod
			if err := r.injectTask(ctx, task, pod.Name); err != nil {
				logger.Error(err, "Failed to inject task into warm pod", "pod", pod.Name)
				_ = r.Delete(ctx, pod)
			} else {
				logger.Info("Claimed warm pod", "pod", pod.Name)
				emitTaskEvent(ctx, "hortator.task.started", task)
				r.Recorder.Event(task, corev1.EventTypeNormal, "TaskStarted", "Agent pod created: "+pod.Name)

				task.Status.Phase = corev1alpha1.AgentTaskPhaseRunning
				task.Status.PodName = pod.Name
				now := metav1.Now()
				task.Status.StartedAt = &now
				task.Status.Message = "Task running (warm pod)"
				tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseRunning), task.Namespace).Inc()
				tasksActive.WithLabelValues(task.Namespace).Inc()
				if err := r.updateStatusWithRetry(ctx, task); err != nil {
					return ctrl.Result{}, err
				}

				// Replenish pool in background
				go func() {
					if err := r.replenishWarmPool(context.Background()); err != nil {
						logger.Error(err, "Failed to replenish warm pool")
					}
				}()

				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
		}
	}

	// Ensure worker ServiceAccount + RBAC exist in the task's namespace
	if err := r.ensureWorkerRBAC(ctx, task.Namespace); err != nil {
		logger.Error(err, "Failed to ensure worker RBAC")
		return ctrl.Result{}, err
	}

	// Create PVC
	if err := r.ensurePVC(ctx, task); err != nil {
		logger.Error(err, "Failed to ensure PVC")
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = fmt.Sprintf("Failed to create PVC: %v", err)
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Collect policies for pod builder injection
	policyList := &corev1alpha1.AgentPolicyList{}
	if err := r.List(ctx, policyList, client.InNamespace(task.Namespace)); err != nil {
		logger.Error(err, "Failed to list policies for pod builder")
	}

	// Create the pod
	pod, err := r.buildPod(ctx, task, policyList.Items...)
	if err != nil {
		logger.Error(err, "Failed to build pod spec")
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = fmt.Sprintf("Failed to build pod: %v", err)
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if err := controllerutil.SetControllerReference(task, pod, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, pod); err != nil {
		if errors.IsAlreadyExists(err) {
			task.Status.Phase = corev1alpha1.AgentTaskPhaseRunning
			task.Status.PodName = pod.Name
			now := metav1.Now()
			task.Status.StartedAt = &now
			task.Status.Message = "Task running"
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseRunning), task.Namespace).Inc()
			tasksActive.WithLabelValues(task.Namespace).Inc()
			if err := r.updateStatusWithRetry(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Created pod", "pod", pod.Name)
	emitTaskEvent(ctx, "hortator.task.started", task)
	r.Recorder.Event(task, corev1.EventTypeNormal, "TaskStarted", "Agent pod created: "+pod.Name)

	task.Status.Phase = corev1alpha1.AgentTaskPhaseRunning
	task.Status.PodName = pod.Name
	now := metav1.Now()
	task.Status.StartedAt = &now
	task.Status.Message = "Task running"
	tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseRunning), task.Namespace).Inc()
	tasksActive.WithLabelValues(task.Namespace).Inc()
	if err := r.updateStatusWithRetry(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleRunning monitors a running task.
func (r *AgentTaskReconciler) handleRunning(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if task.Status.PodName == "" {
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = "Pod name missing"
		setCompletionStatus(task)
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		tasksActive.WithLabelValues(task.Namespace).Dec()
		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	pod := &corev1.Pod{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: task.Namespace,
		Name:      task.Status.PodName,
	}, pod); err != nil {
		if errors.IsNotFound(err) {
			task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
			task.Status.Message = "Pod was deleted"
			setCompletionStatus(task)
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
			tasksActive.WithLabelValues(task.Namespace).Dec()
			if err := r.updateStatusWithRetry(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		logger.Info("Pod succeeded", "pod", pod.Name)

		if task.Status.Output == "" {
			task.Status.Output = r.collectPodLogs(ctx, task.Namespace, task.Status.PodName)
			r.extractTokenUsage(task)
			r.extractResult(task)
		}

		// Calculate estimated cost if price map is available
		if r.PriceMap != nil {
			if costStr := r.PriceMap.CalculateTaskCost(task); costStr != "" {
				task.Status.EstimatedCostUsd = costStr
			}
		}

		// Detect "budget_exceeded" status from runtime result
		if r.isBudgetExceededResult(task) {
			logger.Info("Task exceeded budget", "task", task.Name)
			r.recordAttempt(task, nil, "budget exceeded")
			task.Status.Phase = corev1alpha1.AgentTaskPhaseBudgetExceeded
			task.Status.Message = "Task exceeded token or cost budget"
			setCompletionStatus(task)
			emitTaskEvent(ctx, "hortator.task.budget_exceeded", task, terminalEventAttrs(task)...)
			r.Recorder.Event(task, corev1.EventTypeWarning, "BudgetExceeded", "Task exceeded budget")
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseBudgetExceeded), task.Namespace).Inc()
			budgetExceededTotal.WithLabelValues(task.Namespace).Inc()
			tasksActive.WithLabelValues(task.Namespace).Dec()
			if err := r.updateStatusWithRetry(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			r.updateHierarchyBudget(ctx, task)
			r.notifyParentTask(ctx, task)
			return ctrl.Result{}, nil
		}

		// Agentic tiers: check if the runtime exited in "waiting" status
		// (checkpoint saved, pending children). Transition to Waiting phase
		// instead of Completed — the pod will be re-created when children finish.
		if isAgenticTier(task.Spec.Tier) && r.isWaitingResult(task) {
			logger.Info("Agentic task entering Waiting phase", "task", task.Name)
			r.recordAttempt(task, nil, "waiting for children")

			task.Status.Phase = corev1alpha1.AgentTaskPhaseWaiting
			task.Status.PodName = "" // Pod terminated, PVC persists
			task.Status.Message = "Waiting for child tasks to complete"
			// PendingChildren populated from child task names in status
			emitTaskEvent(ctx, "hortator.task.waiting", task)
			r.Recorder.Event(task, corev1.EventTypeNormal, "TaskWaiting", "Agent checkpointed, waiting for children")

			tasksActive.WithLabelValues(task.Namespace).Dec()
			if err := r.updateStatusWithRetry(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		r.recordAttempt(task, nil, "completed")

		task.Status.Phase = corev1alpha1.AgentTaskPhaseCompleted
		task.Status.Message = "Task completed successfully"
		setCompletionStatus(task)
		emitTaskEvent(ctx, "hortator.task.completed", task, terminalEventAttrs(task)...)
		r.Recorder.Eventf(task, corev1.EventTypeNormal, "TaskCompleted", "Task completed in %s", task.Status.Duration)

		// Store result in cache for deduplication of future identical tasks.
		if r.ResultCache != nil && !shouldSkipCache(task) {
			modelName := ""
			if task.Spec.Model != nil {
				modelName = task.Spec.Model.Name
			}
			cacheKey := CacheKey(task.Spec.Prompt, task.Spec.Role, modelName, task.Spec.Tier)
			var tokensIn, tokensOut int64
			if task.Status.TokensUsed != nil {
				tokensIn = task.Status.TokensUsed.Input
				tokensOut = task.Status.TokensUsed.Output
			}
			r.ResultCache.Put(cacheKey, &CacheResult{
				Output:    task.Status.Output,
				TokensIn:  tokensIn,
				TokensOut: tokensOut,
			})
		}

		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseCompleted), task.Namespace).Inc()
		tasksActive.WithLabelValues(task.Namespace).Dec()
		if task.Status.StartedAt != nil && task.Status.CompletedAt != nil {
			taskDuration.Observe(task.Status.CompletedAt.Time.Sub(task.Status.StartedAt.Time).Seconds())
		}

		// Record cost metric
		r.recordCostMetric(task)

		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}

		// Update hierarchy budget on the root task
		r.updateHierarchyBudget(ctx, task)

		r.notifyParentTask(ctx, task)
		return ctrl.Result{}, nil

	case corev1.PodFailed:
		logger.Info("Pod failed", "pod", pod.Name)

		failureReason := "Task failed"
		var agentExitCode *int32
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Name == "agent" && cs.State.Terminated != nil {
				agentExitCode = &cs.State.Terminated.ExitCode
				failureReason = fmt.Sprintf("Task failed: %s (exit %d)", cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
			}
		}

		// Agent succeeded but sidecar failed
		if agentExitCode != nil && *agentExitCode == 0 {
			logger.Info("Agent succeeded but sidecar failed, treating as success", "pod", pod.Name)
			task.Status.Output = r.collectPodLogs(ctx, task.Namespace, task.Status.PodName)
			r.extractTokenUsage(task)
			r.extractResult(task)
			task.Status.Phase = corev1alpha1.AgentTaskPhaseCompleted
			task.Status.Message = "Task completed (sidecar failed)"
			setCompletionStatus(task)
			r.recordAttempt(task, agentExitCode, "completed (sidecar failed)")
			emitTaskEvent(ctx, "hortator.task.completed", task, terminalEventAttrs(task)...)
			r.Recorder.Eventf(task, corev1.EventTypeNormal, "TaskCompleted", "Task completed in %s", task.Status.Duration)
			tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseCompleted), task.Namespace).Inc()
			tasksActive.WithLabelValues(task.Namespace).Dec()
			if err := r.updateStatusWithRetry(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			r.notifyParentTask(ctx, task)
			return ctrl.Result{}, nil
		}

		task.Status.Output = r.collectPodLogs(ctx, task.Namespace, task.Status.PodName)

		isTransient := r.isTransientFailure(ctx, task, pod)
		r.recordAttempt(task, agentExitCode, failureReason)

		if isTransient && r.shouldRetry(task) {
			backoff := r.computeBackoff(task)
			nextRetry := metav1.NewTime(time.Now().Add(backoff))
			task.Status.Phase = corev1alpha1.AgentTaskPhaseRetrying
			task.Status.NextRetryTime = &nextRetry
			task.Status.PodName = ""
			task.Status.Message = fmt.Sprintf("Retrying in %s (attempt %d/%d): %s",
				backoff.Round(time.Second), task.Status.Attempts, task.Spec.Retry.MaxAttempts, failureReason)
			emitTaskEvent(ctx, "hortator.task.retrying", task)
			r.Recorder.Eventf(task, corev1.EventTypeNormal, "TaskRetrying", "Retrying (attempt %d/%d)", task.Status.Attempts, task.Spec.Retry.MaxAttempts)
			logger.Info("Scheduling retry", "attempt", task.Status.Attempts, "backoff", backoff)

			if err := r.updateStatusWithRetry(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: backoff}, nil
		}

		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = failureReason
		setCompletionStatus(task)
		emitTaskEvent(ctx, "hortator.task.failed", task, terminalEventAttrs(task)...)
		r.Recorder.Event(task, corev1.EventTypeWarning, "TaskFailed", "Task failed: "+failureReason)

		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		tasksActive.WithLabelValues(task.Namespace).Dec()
		if task.Status.StartedAt != nil && task.Status.CompletedAt != nil {
			taskDuration.Observe(task.Status.CompletedAt.Time.Sub(task.Status.StartedAt.Time).Seconds())
		}

		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}

		r.updateHierarchyBudget(ctx, task)
		r.notifyParentTask(ctx, task)
		return ctrl.Result{}, nil

	case corev1.PodPending, corev1.PodRunning:
		if task.Status.StartedAt != nil && task.Spec.Timeout != nil {
			timeout := time.Duration(*task.Spec.Timeout) * time.Second
			elapsed := time.Since(task.Status.StartedAt.Time)
			if elapsed > timeout {
				logger.Info("Task timed out", "elapsed", elapsed, "timeout", timeout)
				if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				task.Status.Phase = corev1alpha1.AgentTaskPhaseTimedOut
				task.Status.Message = fmt.Sprintf("Task timed out after %s", timeout)
				setCompletionStatus(task)
				emitTaskEvent(ctx, "hortator.task.failed", task, terminalEventAttrs(task)...)
				r.Recorder.Event(task, corev1.EventTypeWarning, "TaskFailed", "Task failed: "+task.Status.Message)
				tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseTimedOut), task.Namespace).Inc()
				tasksActive.WithLabelValues(task.Namespace).Dec()
				if err := r.updateStatusWithRetry(ctx, task); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
		}

		// ── Stuck detection (only for running pods) ───────────────────
		if pod.Status.Phase == corev1.PodRunning {
			r.defaultsMu.RLock()
			healthCfg := r.defaults.Health
			r.defaultsMu.RUnlock()

			if healthCfg.Enabled && healthCfg.StuckDetection.Enabled {
				roleHealth := r.fetchRoleHealth(ctx, task)
				cfg := resolveStuckConfig(healthCfg.StuckDetection, roleHealth, task)
				score := r.checkStuckSignals(ctx, task, pod, cfg)

				// Update tool diversity metric
				taskToolDiversity.WithLabelValues(task.Namespace, task.Name).Set(score.ToolDiversity)

				if score.IsStuck {
					if err := r.executeStuckAction(ctx, task, pod, score, cfg.Action); err != nil {
						return ctrl.Result{}, err
					}
					// If action was kill or escalate, the task is now terminal
					if cfg.Action == "kill" || cfg.Action == "escalate" {
						return ctrl.Result{}, nil
					}
				}
			}
		}

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil

	default:
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
}

// handleRetrying checks if it's time to retry and transitions back to Pending.
func (r *AgentTaskReconciler) handleRetrying(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	if task.Status.NextRetryTime == nil {
		task.Status.Phase = corev1alpha1.AgentTaskPhasePending
		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	remaining := time.Until(task.Status.NextRetryTime.Time)
	if remaining > 0 {
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	log.FromContext(ctx).Info("Retrying task", "task", task.Name, "attempt", task.Status.Attempts+1)
	task.Status.Phase = corev1alpha1.AgentTaskPhasePending
	task.Status.NextRetryTime = nil
	task.Status.Message = fmt.Sprintf("Retry attempt %d", task.Status.Attempts+1)
	if err := r.updateStatusWithRetry(ctx, task); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// isTransientFailure classifies a pod failure as transient or logical.
func (r *AgentTaskReconciler) isTransientFailure(ctx context.Context, task *corev1alpha1.AgentTask, pod *corev1.Pod) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == "agent" && cs.State.Terminated != nil {
			if cs.State.Terminated.ExitCode == 0 {
				return false
			}
		}
	}
	return true
}

// shouldRetry returns true if the task has retries configured and hasn't exhausted them.
func (r *AgentTaskReconciler) shouldRetry(task *corev1alpha1.AgentTask) bool {
	if task.Spec.Retry == nil || task.Spec.Retry.MaxAttempts <= 0 {
		return false
	}
	return task.Status.Attempts < task.Spec.Retry.MaxAttempts
}

// computeBackoff returns the backoff duration for the current attempt with ±25% jitter.
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

// handleWaiting monitors a task in the Waiting phase.
// The task is idle (no pod running). It re-checks on a timer in case child
// completion events were missed. The primary wake-up path is notifyParentTask.
func (r *AgentTaskReconciler) handleWaiting(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check if all pending children have reached a terminal phase
	allDone := true
	for _, childName := range task.Status.PendingChildren {
		child := &corev1alpha1.AgentTask{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: task.Namespace,
			Name:      childName,
		}, child); err != nil {
			logger.V(1).Info("Failed to fetch pending child", "child", childName, "error", err)
			allDone = false
			continue
		}
		if !isTerminalPhase(child.Status.Phase) {
			allDone = false
		}
	}

	if allDone && len(task.Status.PendingChildren) > 0 {
		logger.Info("All children complete, reincarnating parent", "task", task.Name)
		task.Status.Phase = corev1alpha1.AgentTaskPhasePending
		task.Status.PendingChildren = nil // Clear for the next run
		task.Status.Message = "Children completed, restarting agent"
		r.Recorder.Event(task, corev1.EventTypeNormal, "TaskReincarnating", "All children done, restarting agent pod")
		if err := r.updateStatusWithRetry(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Poll periodically in case we missed a child completion event
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// isBudgetExceededResult returns true if the runtime reported a budget_exceeded status.
func (r *AgentTaskReconciler) isBudgetExceededResult(task *corev1alpha1.AgentTask) bool {
	if strings.Contains(task.Status.Output, `"status": "budget_exceeded"`) ||
		strings.Contains(task.Status.Output, `"status":"budget_exceeded"`) {
		return true
	}
	// Also check if the operator-side budget calculation exceeds the task limit
	if r.PriceMap != nil && task.Status.EstimatedCostUsd != "" {
		cost, err := strconv.ParseFloat(task.Status.EstimatedCostUsd, 64)
		if err == nil && IsBudgetExceeded(task, cost) {
			return true
		}
	}
	return false
}

// recordCostMetric records the task cost as a Prometheus metric.
func (r *AgentTaskReconciler) recordCostMetric(task *corev1alpha1.AgentTask) {
	if task.Status.EstimatedCostUsd == "" {
		return
	}
	cost, err := strconv.ParseFloat(task.Status.EstimatedCostUsd, 64)
	if err == nil && cost > 0 {
		taskCostUsd.Observe(cost)
	}
}

// isWaitingResult returns true if the agentic runtime wrote a "waiting" status
// to result.json, meaning it checkpointed and is waiting for children.
func (r *AgentTaskReconciler) isWaitingResult(task *corev1alpha1.AgentTask) bool {
	// The runtime reports "waiting" status via the hortator report CLI or
	// we detect it from the output which contains the status.
	return strings.Contains(task.Status.Output, `"status": "waiting"`) ||
		strings.Contains(task.Status.Output, `"status":"waiting"`) ||
		strings.Contains(task.Status.Message, "Waiting") ||
		len(task.Status.PendingChildren) > 0
}

// isTerminalPhase returns true if the phase is a terminal (done) state.
func isTerminalPhase(phase corev1alpha1.AgentTaskPhase) bool {
	switch phase {
	case corev1alpha1.AgentTaskPhaseCompleted,
		corev1alpha1.AgentTaskPhaseFailed,
		corev1alpha1.AgentTaskPhaseTimedOut,
		corev1alpha1.AgentTaskPhaseBudgetExceeded,
		corev1alpha1.AgentTaskPhaseCancelled:
		return true
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Clean up orphaned warm pool resources on startup
	if r.defaults.WarmPool.Enabled {
		ctx := context.Background()
		if err := r.cleanupOrphanedWarmResources(ctx); err != nil {
			log.FromContext(ctx).Error(err, "Failed to clean up orphaned warm resources on startup")
		}
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.AgentTask{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
