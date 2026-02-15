/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package webhook

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var agenttasklog = logf.Log.WithName("agenttask-webhook")

// AgentTaskValidator validates AgentTask resources.
type AgentTaskValidator struct {
	Client client.Client
}

// tierRank returns the numeric rank for a tier (higher = more privileged).
func tierRank(tier string) int {
	switch tier {
	case "legionary":
		return 0
	case "centurion":
		return 1
	case "tribune":
		return 2
	default:
		return 0
	}
}

// ValidateAgentTask performs cross-field validation on an AgentTask.
// Exported for unit testing without needing a webhook server.
func ValidateAgentTask(task *corev1alpha1.AgentTask, parent *corev1alpha1.AgentTask) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// model.name and model.endpoint are required
	if task.Spec.Model == nil {
		allErrs = append(allErrs, field.Required(specPath.Child("model"), "model is required"))
	} else {
		if task.Spec.Model.Name == "" {
			allErrs = append(allErrs, field.Required(specPath.Child("model", "name"), "model name is required"))
		}
		if task.Spec.Model.Endpoint == "" {
			allErrs = append(allErrs, field.Required(specPath.Child("model", "endpoint"), "model endpoint is required"))
		}
	}

	// timeout must be > 0 if set
	if task.Spec.Timeout != nil && *task.Spec.Timeout <= 0 {
		allErrs = append(allErrs, field.Invalid(specPath.Child("timeout"), *task.Spec.Timeout, "timeout must be > 0"))
	}

	// budget.maxTokens must be > 0 if set
	if task.Spec.Budget != nil {
		if task.Spec.Budget.MaxTokens != nil && *task.Spec.Budget.MaxTokens <= 0 {
			allErrs = append(allErrs, field.Invalid(specPath.Child("budget", "maxTokens"),
				*task.Spec.Budget.MaxTokens, "maxTokens must be > 0"))
		}
		if task.Spec.Budget.MaxCostUsd != "" {
			cost, err := strconv.ParseFloat(task.Spec.Budget.MaxCostUsd, 64)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(specPath.Child("budget", "maxCostUsd"),
					task.Spec.Budget.MaxCostUsd, "must be a valid number"))
			} else if cost <= 0 {
				allErrs = append(allErrs, field.Invalid(specPath.Child("budget", "maxCostUsd"),
					task.Spec.Budget.MaxCostUsd, "maxCostUsd must be > 0"))
			}
		}
	}

	// Parent-child constraints
	if parent != nil {
		// Child tier must be <= parent tier
		if tierRank(task.Spec.Tier) > tierRank(parent.Spec.Tier) {
			allErrs = append(allErrs, field.Forbidden(specPath.Child("tier"),
				fmt.Sprintf("child tier %q exceeds parent tier %q", task.Spec.Tier, parent.Spec.Tier)))
		}

		// Child budget must not exceed parent budget
		if task.Spec.Budget != nil && parent.Spec.Budget != nil {
			if task.Spec.Budget.MaxTokens != nil && parent.Spec.Budget.MaxTokens != nil {
				if *task.Spec.Budget.MaxTokens > *parent.Spec.Budget.MaxTokens {
					allErrs = append(allErrs, field.Forbidden(specPath.Child("budget", "maxTokens"),
						"child maxTokens exceeds parent maxTokens"))
				}
			}
			if task.Spec.Budget.MaxCostUsd != "" && parent.Spec.Budget.MaxCostUsd != "" {
				childCost, err1 := strconv.ParseFloat(task.Spec.Budget.MaxCostUsd, 64)
				parentCost, err2 := strconv.ParseFloat(parent.Spec.Budget.MaxCostUsd, 64)
				if err1 == nil && err2 == nil && childCost > parentCost {
					allErrs = append(allErrs, field.Forbidden(specPath.Child("budget", "maxCostUsd"),
						"child maxCostUsd exceeds parent maxCostUsd"))
				}
			}
		}

		// Child capabilities must be a subset of parent capabilities
		parentCaps := make(map[string]bool, len(parent.Spec.Capabilities))
		// Include auto-injected spawn for agentic tiers
		for _, c := range parent.Spec.Capabilities {
			parentCaps[c] = true
		}
		if parent.Spec.Tier == "tribune" || parent.Spec.Tier == "centurion" {
			parentCaps["spawn"] = true
		}
		for _, cap := range task.Spec.Capabilities {
			if !parentCaps[cap] {
				allErrs = append(allErrs, field.Forbidden(specPath.Child("capabilities"),
					fmt.Sprintf("child capability %q not in parent capabilities %v",
						cap, capKeys(parentCaps))))
			}
		}
	}

	return allErrs
}

func capKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// +kubebuilder:webhook:path=/validate-core-hortator-ai-v1alpha1-agenttask,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.hortator.ai,resources=agenttasks,verbs=create;update,versions=v1alpha1,name=vagenttask.kb.io,admissionReviewVersions=v1

// ValidateCreate implements webhook.CustomValidator.
func (v *AgentTaskValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	task, ok := obj.(*corev1alpha1.AgentTask)
	if !ok {
		return nil, fmt.Errorf("expected AgentTask, got %T", obj)
	}
	agenttasklog.Info("validate create", "name", task.Name)

	var parent *corev1alpha1.AgentTask
	if task.Spec.ParentTaskID != "" {
		parent = &corev1alpha1.AgentTask{}
		if err := v.Client.Get(ctx, client.ObjectKey{
			Namespace: task.Namespace,
			Name:      task.Spec.ParentTaskID,
		}, parent); err != nil {
			return nil, fmt.Errorf("failed to fetch parent task %s: %w", task.Spec.ParentTaskID, err)
		}
	}

	errs := ValidateAgentTask(task, parent)
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("validation failed: %s", strings.Join(msgs, "; "))
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *AgentTaskValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	task, ok := newObj.(*corev1alpha1.AgentTask)
	if !ok {
		return nil, fmt.Errorf("expected AgentTask, got %T", newObj)
	}
	agenttasklog.Info("validate update", "name", task.Name)

	var parent *corev1alpha1.AgentTask
	if task.Spec.ParentTaskID != "" {
		parent = &corev1alpha1.AgentTask{}
		if err := v.Client.Get(ctx, client.ObjectKey{
			Namespace: task.Namespace,
			Name:      task.Spec.ParentTaskID,
		}, parent); err != nil {
			return nil, fmt.Errorf("failed to fetch parent task %s: %w", task.Spec.ParentTaskID, err)
		}
	}

	errs := ValidateAgentTask(task, parent)
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("validation failed: %s", strings.Join(msgs, "; "))
	}
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator.
func (v *AgentTaskValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// SetupWebhookWithManager registers the validating webhook with the manager.
func (v *AgentTaskValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&corev1alpha1.AgentTask{}).
		WithValidator(v).
		Complete()
}
