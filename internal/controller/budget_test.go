/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func TestIsBudgetExceeded_NoBudget(t *testing.T) {
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
		},
	}
	if IsBudgetExceeded(task, 10.0) {
		t.Error("expected no budget exceeded when budget is nil")
	}
}

func TestIsBudgetExceeded_TokenLimit(t *testing.T) {
	maxTokens := int64(100)
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Budget: &corev1alpha1.BudgetSpec{
				MaxTokens: &maxTokens,
			},
		},
		Status: corev1alpha1.AgentTaskStatus{
			TokensUsed: &corev1alpha1.TokenUsage{
				Input:  80,
				Output: 30,
			},
		},
	}
	if !IsBudgetExceeded(task, 0) {
		t.Error("expected budget exceeded when tokens (110) > maxTokens (100)")
	}
}

func TestIsBudgetExceeded_TokenLimit_NotExceeded(t *testing.T) {
	maxTokens := int64(100)
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Budget: &corev1alpha1.BudgetSpec{
				MaxTokens: &maxTokens,
			},
		},
		Status: corev1alpha1.AgentTaskStatus{
			TokensUsed: &corev1alpha1.TokenUsage{
				Input:  30,
				Output: 20,
			},
		},
	}
	if IsBudgetExceeded(task, 0) {
		t.Error("expected budget NOT exceeded when tokens (50) < maxTokens (100)")
	}
}

func TestIsBudgetExceeded_CostLimit(t *testing.T) {
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Budget: &corev1alpha1.BudgetSpec{
				MaxCostUsd: "0.50",
			},
		},
	}
	if !IsBudgetExceeded(task, 0.75) {
		t.Error("expected budget exceeded when cost ($0.75) > maxCostUsd ($0.50)")
	}
}

func TestIsBudgetExceeded_CostLimit_NotExceeded(t *testing.T) {
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Budget: &corev1alpha1.BudgetSpec{
				MaxCostUsd: "1.00",
			},
		},
	}
	if IsBudgetExceeded(task, 0.50) {
		t.Error("expected budget NOT exceeded when cost ($0.50) < maxCostUsd ($1.00)")
	}
}

func TestIsBudgetExceeded_BothLimits(t *testing.T) {
	maxTokens := int64(1000)
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Budget: &corev1alpha1.BudgetSpec{
				MaxTokens:  &maxTokens,
				MaxCostUsd: "0.50",
			},
		},
		Status: corev1alpha1.AgentTaskStatus{
			TokensUsed: &corev1alpha1.TokenUsage{
				Input:  100,
				Output: 50,
			},
		},
	}
	// Tokens under, cost over
	if !IsBudgetExceeded(task, 0.75) {
		t.Error("expected budget exceeded when cost exceeds limit (even if tokens don't)")
	}
}

func TestPriceMap_CalculateCost(t *testing.T) {
	pm := NewPriceMap(24)
	pm.prices = map[string]ModelPricing{
		"claude-sonnet-4-20250514": {
			InputCostPerToken:  0.000003,
			OutputCostPerToken: 0.000015,
		},
	}

	cost, err := pm.CalculateCost("claude-sonnet-4-20250514", 1000, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 1000*0.000003 + 500*0.000015
	if abs(cost-expected) > 0.000001 {
		t.Errorf("expected cost=%.6f, got %.6f", expected, cost)
	}
}

func TestPriceMap_CalculateCost_UnknownModel(t *testing.T) {
	pm := NewPriceMap(24)
	pm.prices = map[string]ModelPricing{}

	_, err := pm.CalculateCost("nonexistent-model", 1000, 500)
	if err == nil {
		t.Error("expected error for unknown model")
	}
}

func TestPriceMap_PrefixLookup(t *testing.T) {
	pm := NewPriceMap(24)
	pm.prices = map[string]ModelPricing{
		"anthropic/claude-sonnet-4-20250514": {
			InputCostPerToken:  0.000003,
			OutputCostPerToken: 0.000015,
		},
	}

	// Should find via prefix fallback
	pricing, ok := pm.GetPricing("claude-sonnet-4-20250514")
	if !ok {
		t.Error("expected to find pricing via anthropic/ prefix fallback")
	}
	if pricing.InputCostPerToken != 0.000003 {
		t.Errorf("unexpected input cost: %f", pricing.InputCostPerToken)
	}
}
