/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"fmt"
	"path"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

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

// collectShellPolicy aggregates shell command filtering and read-only workspace
// settings from all matching AgentPolicy objects in the task's namespace.
// Returns allowed commands, denied commands, and whether workspace should be read-only.
func (r *AgentTaskReconciler) collectShellPolicy(ctx context.Context, namespace string) (allowed, denied []string, readOnlyWorkspace bool) {
	policies := &corev1alpha1.AgentPolicyList{}
	if err := r.List(ctx, policies, client.InNamespace(namespace)); err != nil {
		return nil, nil, false
	}

	seenAllowed := map[string]bool{}
	seenDenied := map[string]bool{}

	for _, policy := range policies.Items {
		p := policy.Spec
		for _, cmd := range p.AllowedShellCommands {
			if !seenAllowed[cmd] {
				seenAllowed[cmd] = true
				allowed = append(allowed, cmd)
			}
		}
		for _, cmd := range p.DeniedShellCommands {
			if !seenDenied[cmd] {
				seenDenied[cmd] = true
				denied = append(denied, cmd)
			}
		}
		if p.ReadOnlyWorkspace {
			readOnlyWorkspace = true
		}
	}

	return allowed, denied, readOnlyWorkspace
}
