/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"fmt"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

const maxHierarchyDepth = 10

// findRootTask walks the ParentTaskID chain up to the root task.
// Returns the root task (which may be the task itself if it has no parent).
// Returns an error if the chain exceeds maxHierarchyDepth or a parent is not found.
func (r *AgentTaskReconciler) findRootTask(ctx context.Context, task *corev1alpha1.AgentTask) (*corev1alpha1.AgentTask, error) {
	current := task
	for i := 0; i < maxHierarchyDepth; i++ {
		if current.Spec.ParentTaskID == "" {
			return current, nil
		}
		parent := &corev1alpha1.AgentTask{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: current.Namespace,
			Name:      current.Spec.ParentTaskID,
		}, parent); err != nil {
			return nil, fmt.Errorf("failed to fetch parent %s: %w", current.Spec.ParentTaskID, err)
		}
		current = parent
	}
	return nil, fmt.Errorf("hierarchy depth exceeded max of %d", maxHierarchyDepth)
}

// checkHierarchyBudgetExhausted checks if the root task's hierarchy budget is already exhausted.
// Returns a non-empty reason string if exhausted, empty string if OK or no hierarchy budget.
func (r *AgentTaskReconciler) checkHierarchyBudgetExhausted(ctx context.Context, task *corev1alpha1.AgentTask) string {
	if task.Spec.ParentTaskID == "" {
		return ""
	}

	root, err := r.findRootTask(ctx, task)
	if err != nil {
		log.FromContext(ctx).V(1).Info("Failed to find root task for hierarchy budget check", "error", err)
		return ""
	}

	if root.Spec.HierarchyBudget == nil {
		return ""
	}

	return isHierarchyBudgetExceeded(root)
}

// isHierarchyBudgetExceeded checks if a root task's hierarchy budget has been exceeded.
// Returns a reason string if exceeded, empty string otherwise.
func isHierarchyBudgetExceeded(root *corev1alpha1.AgentTask) string {
	budget := root.Spec.HierarchyBudget
	if budget == nil {
		return ""
	}

	// Check token limit
	if budget.MaxTokens != nil && root.Status.HierarchyTokensUsed != nil {
		totalTokens := root.Status.HierarchyTokensUsed.Input + root.Status.HierarchyTokensUsed.Output
		if totalTokens >= *budget.MaxTokens {
			return fmt.Sprintf("hierarchy token budget exhausted: %d/%d", totalTokens, *budget.MaxTokens)
		}
	}

	// Check cost limit
	if budget.MaxCostUsd != "" && root.Status.HierarchyCostUsed != "" {
		maxCost, err1 := strconv.ParseFloat(budget.MaxCostUsd, 64)
		usedCost, err2 := strconv.ParseFloat(root.Status.HierarchyCostUsed, 64)
		if err1 == nil && err2 == nil && usedCost >= maxCost {
			return fmt.Sprintf("hierarchy cost budget exhausted: $%.4f/$%.4f", usedCost, maxCost)
		}
	}

	return ""
}

// updateHierarchyBudget adds a completed task's usage to the root task's hierarchy budget tracking.
// If the budget is exceeded after the update, it cancels all pending/running descendants.
func (r *AgentTaskReconciler) updateHierarchyBudget(ctx context.Context, task *corev1alpha1.AgentTask) {
	if task.Spec.ParentTaskID == "" && task.Spec.HierarchyBudget == nil {
		return // Not part of a hierarchy, or root without hierarchy budget
	}

	logger := log.FromContext(ctx)

	root, err := r.findRootTask(ctx, task)
	if err != nil {
		logger.V(1).Info("Failed to find root task for hierarchy budget update", "error", err)
		return
	}

	if root.Spec.HierarchyBudget == nil {
		return
	}

	// Don't update root's budget from its own usage here — only from descendants.
	// The root's own usage is also tracked to get the full picture.
	// Initialize hierarchy tracking fields if needed.
	if root.Status.HierarchyTokensUsed == nil {
		root.Status.HierarchyTokensUsed = &corev1alpha1.TokenUsage{}
	}

	// Add this task's token usage
	if task.Status.TokensUsed != nil {
		root.Status.HierarchyTokensUsed.Input += task.Status.TokensUsed.Input
		root.Status.HierarchyTokensUsed.Output += task.Status.TokensUsed.Output
	}

	// Add this task's cost
	if task.Status.EstimatedCostUsd != "" {
		taskCost, err := strconv.ParseFloat(task.Status.EstimatedCostUsd, 64)
		if err == nil {
			existingCost := 0.0
			if root.Status.HierarchyCostUsed != "" {
				existingCost, _ = strconv.ParseFloat(root.Status.HierarchyCostUsed, 64)
			}
			root.Status.HierarchyCostUsed = fmt.Sprintf("%.6f", existingCost+taskCost)
		}
	}

	// Persist the updated root status
	if err := r.updateStatusWithRetry(ctx, root); err != nil {
		logger.V(1).Info("Failed to update root hierarchy budget", "root", root.Name, "error", err)
		return
	}

	// Check if budget is now exceeded
	if reason := isHierarchyBudgetExceeded(root); reason != "" {
		logger.Info("Hierarchy budget exceeded, cancelling descendants", "root", root.Name, "reason", reason)
		r.cancelDescendants(ctx, root, reason)
	}
}

// cancelDescendants cancels all pending/running tasks that are descendants of the given root.
func (r *AgentTaskReconciler) cancelDescendants(ctx context.Context, root *corev1alpha1.AgentTask, reason string) {
	logger := log.FromContext(ctx)

	// List all tasks in the namespace
	taskList := &corev1alpha1.AgentTaskList{}
	if err := r.List(ctx, taskList, client.InNamespace(root.Namespace)); err != nil {
		logger.V(1).Info("Failed to list tasks for hierarchy cancellation", "error", err)
		return
	}

	// Build a set of all task names in this tree
	treeMembers := map[string]bool{root.Name: true}
	// Multiple passes to catch full tree (parent might be listed after child)
	for pass := 0; pass < maxHierarchyDepth; pass++ {
		changed := false
		for i := range taskList.Items {
			t := &taskList.Items[i]
			if t.Spec.ParentTaskID != "" && treeMembers[t.Spec.ParentTaskID] && !treeMembers[t.Name] {
				treeMembers[t.Name] = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Cancel non-terminal tasks in the tree (except root itself)
	for i := range taskList.Items {
		t := &taskList.Items[i]
		if t.Name == root.Name {
			continue
		}
		if !treeMembers[t.Name] {
			continue
		}
		if isTerminalPhase(t.Status.Phase) {
			continue
		}

		logger.Info("Cancelling task due to hierarchy budget", "task", t.Name)
		t.Status.Phase = corev1alpha1.AgentTaskPhaseCancelled
		t.Status.Message = fmt.Sprintf("Hierarchy budget exceeded: %s", reason)
		setCompletionStatus(t)
		if err := r.updateStatusWithRetry(ctx, t); err != nil {
			logger.V(1).Info("Failed to cancel descendant", "task", t.Name, "error", err)
		}
	}
}
