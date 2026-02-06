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
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

const (
	finalizerName = "agenttask.core.hortator.ai/finalizer"
)

// AgentTaskReconciler reconciles a AgentTask object
type AgentTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.hortator.ai,resources=agenttasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.hortator.ai,resources=agenttasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.hortator.ai,resources=agenttasks/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get

// Reconcile is the main reconciliation loop for AgentTask resources
func (r *AgentTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

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
		if err := r.Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize status if needed
	if task.Status.Phase == "" {
		task.Status.Phase = corev1alpha1.AgentTaskPhasePending
		task.Status.Message = "Task pending"
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
	case corev1alpha1.AgentTaskPhaseSucceeded, corev1alpha1.AgentTaskPhaseFailed:
		// Terminal state, nothing to do
		return ctrl.Result{}, nil
	default:
		logger.Info("Unknown phase", "phase", task.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// handleDeletion handles cleanup when task is deleted
func (r *AgentTaskReconciler) handleDeletion(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(task, finalizerName) {
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

		// Remove finalizer
		controllerutil.RemoveFinalizer(task, finalizerName)
		if err := r.Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// handlePending creates the pod for a pending task
func (r *AgentTaskReconciler) handlePending(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Create the pod
	pod, err := r.buildPod(task)
	if err != nil {
		logger.Error(err, "Failed to build pod spec")
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = fmt.Sprintf("Failed to build pod: %v", err)
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
			task.Status.StartTime = &now
			task.Status.Message = "Task running"
			if err := r.Status().Update(ctx, task); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Created pod", "pod", pod.Name)

	// Update status
	task.Status.Phase = corev1alpha1.AgentTaskPhaseRunning
	task.Status.PodName = pod.Name
	now := metav1.Now()
	task.Status.StartTime = &now
	task.Status.Message = "Task running"
	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleRunning monitors a running task
func (r *AgentTaskReconciler) handleRunning(ctx context.Context, task *corev1alpha1.AgentTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if task.Status.PodName == "" {
		// No pod name, something went wrong
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = "Pod name missing"
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
			now := metav1.Now()
			task.Status.CompletionTime = &now
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
		task.Status.Phase = corev1alpha1.AgentTaskPhaseSucceeded
		task.Status.Message = "Task completed successfully"
		now := metav1.Now()
		task.Status.CompletionTime = &now
		// Output would be collected from pod logs by the CLI
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case corev1.PodFailed:
		logger.Info("Pod failed", "pod", pod.Name)
		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = "Task failed"
		now := metav1.Now()
		task.Status.CompletionTime = &now
		if len(pod.Status.ContainerStatuses) > 0 {
			cs := pod.Status.ContainerStatuses[0]
			if cs.State.Terminated != nil {
				task.Status.Message = fmt.Sprintf("Task failed: %s", cs.State.Terminated.Reason)
			}
		}
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	case corev1.PodPending, corev1.PodRunning:
		// Check for timeout
		if task.Status.StartTime != nil && task.Spec.Timeout != "" {
			timeout, err := time.ParseDuration(task.Spec.Timeout)
			if err == nil {
				elapsed := time.Since(task.Status.StartTime.Time)
				if elapsed > timeout {
					logger.Info("Task timed out", "elapsed", elapsed, "timeout", timeout)
					// Delete the pod
					if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
						return ctrl.Result{}, err
					}
					task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
					task.Status.Message = fmt.Sprintf("Task timed out after %s", timeout)
					now := metav1.Now()
					task.Status.CompletionTime = &now
					if err := r.Status().Update(ctx, task); err != nil {
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, nil
				}
			}
		}
		// Continue monitoring
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil

	default:
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
}

// buildPod creates a pod spec for the agent task
func (r *AgentTaskReconciler) buildPod(task *corev1alpha1.AgentTask) (*corev1.Pod, error) {
	image := task.Spec.Image
	if image == "" {
		image = "ghcr.io/hortator-ai/agent:latest"
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

	if task.Spec.Model != "" {
		env = append(env, corev1.EnvVar{
			Name:  "HORTATOR_MODEL",
			Value: task.Spec.Model,
		})
	}

	// Add custom env vars
	for k, v := range task.Spec.Env {
		env = append(env, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	// Build resource requirements
	resources := corev1.ResourceRequirements{}
	if task.Spec.Resources != nil {
		if task.Spec.Resources.CPU != "" {
			if resources.Requests == nil {
				resources.Requests = corev1.ResourceList{}
			}
			resources.Requests[corev1.ResourceCPU] = resource.MustParse(task.Spec.Resources.CPU)
		}
		if task.Spec.Resources.Memory != "" {
			if resources.Requests == nil {
				resources.Requests = corev1.ResourceList{}
			}
			resources.Requests[corev1.ResourceMemory] = resource.MustParse(task.Spec.Resources.Memory)
		}
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
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:      "agent",
					Image:     image,
					Env:       env,
					Resources: resources,
				},
			},
		},
	}

	return pod, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.AgentTask{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}
