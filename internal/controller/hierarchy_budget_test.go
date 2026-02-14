/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func ptr64(v int64) *int64 { return &v }

func TestIsHierarchyBudgetExceeded_NoBudget(t *testing.T) {
	root := &corev1alpha1.AgentTask{}
	if reason := isHierarchyBudgetExceeded(root); reason != "" {
		t.Errorf("expected empty, got %q", reason)
	}
}

func TestIsHierarchyBudgetExceeded_TokensUnderLimit(t *testing.T) {
	root := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			HierarchyBudget: &corev1alpha1.BudgetSpec{
				MaxTokens: ptr64(1000),
			},
		},
		Status: corev1alpha1.AgentTaskStatus{
			HierarchyTokensUsed: &corev1alpha1.TokenUsage{
				Input:  300,
				Output: 200,
			},
		},
	}
	if reason := isHierarchyBudgetExceeded(root); reason != "" {
		t.Errorf("expected empty, got %q", reason)
	}
}

func TestIsHierarchyBudgetExceeded_TokensAtLimit(t *testing.T) {
	root := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			HierarchyBudget: &corev1alpha1.BudgetSpec{
				MaxTokens: ptr64(1000),
			},
		},
		Status: corev1alpha1.AgentTaskStatus{
			HierarchyTokensUsed: &corev1alpha1.TokenUsage{
				Input:  500,
				Output: 500,
			},
		},
	}
	if reason := isHierarchyBudgetExceeded(root); reason == "" {
		t.Error("expected exceeded, got empty")
	}
}

func TestIsHierarchyBudgetExceeded_CostExceeded(t *testing.T) {
	root := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			HierarchyBudget: &corev1alpha1.BudgetSpec{
				MaxCostUsd: "1.00",
			},
		},
		Status: corev1alpha1.AgentTaskStatus{
			HierarchyCostUsed: "1.50",
		},
	}
	if reason := isHierarchyBudgetExceeded(root); reason == "" {
		t.Error("expected exceeded, got empty")
	}
}

func TestIsHierarchyBudgetExceeded_CostUnder(t *testing.T) {
	root := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			HierarchyBudget: &corev1alpha1.BudgetSpec{
				MaxCostUsd: "1.00",
			},
		},
		Status: corev1alpha1.AgentTaskStatus{
			HierarchyCostUsed: "0.50",
		},
	}
	if reason := isHierarchyBudgetExceeded(root); reason != "" {
		t.Errorf("expected empty, got %q", reason)
	}
}

// Suppress unused import warning for metav1
var _ = metav1.ObjectMeta{}
