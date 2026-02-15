/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package webhook

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func intPtr(i int) *int       { return &i }
func int64Ptr(i int64) *int64 { return &i }

func validTask() *corev1alpha1.AgentTask {
	return &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{Name: "child", Namespace: "default"},
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "do something",
			Tier:   "legionary",
			Model: &corev1alpha1.ModelSpec{
				Name:     "claude-sonnet",
				Endpoint: "https://api.anthropic.com/v1",
			},
		},
	}
}

func TestValidateAgentTask_ModelRequired(t *testing.T) {
	task := validTask()
	task.Spec.Model = nil
	errs := ValidateAgentTask(task, nil)
	if len(errs) == 0 {
		t.Error("expected error for missing model")
	}
}

func TestValidateAgentTask_ModelNameRequired(t *testing.T) {
	task := validTask()
	task.Spec.Model.Name = ""
	errs := ValidateAgentTask(task, nil)
	if len(errs) == 0 {
		t.Error("expected error for empty model name")
	}
}

func TestValidateAgentTask_ModelEndpointRequired(t *testing.T) {
	task := validTask()
	task.Spec.Model.Endpoint = ""
	errs := ValidateAgentTask(task, nil)
	if len(errs) == 0 {
		t.Error("expected error for empty model endpoint")
	}
}

func TestValidateAgentTask_TimeoutPositive(t *testing.T) {
	task := validTask()
	task.Spec.Timeout = intPtr(0)
	errs := ValidateAgentTask(task, nil)
	if len(errs) == 0 {
		t.Error("expected error for zero timeout")
	}

	task.Spec.Timeout = intPtr(-1)
	errs = ValidateAgentTask(task, nil)
	if len(errs) == 0 {
		t.Error("expected error for negative timeout")
	}
}

func TestValidateAgentTask_TimeoutNilOK(t *testing.T) {
	task := validTask()
	task.Spec.Timeout = nil
	errs := ValidateAgentTask(task, nil)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateAgentTask_BudgetMaxTokensPositive(t *testing.T) {
	task := validTask()
	task.Spec.Budget = &corev1alpha1.BudgetSpec{MaxTokens: int64Ptr(0)}
	errs := ValidateAgentTask(task, nil)
	if len(errs) == 0 {
		t.Error("expected error for zero maxTokens")
	}
}

func TestValidateAgentTask_BudgetMaxCostPositive(t *testing.T) {
	task := validTask()
	task.Spec.Budget = &corev1alpha1.BudgetSpec{MaxCostUsd: "-1.0"}
	errs := ValidateAgentTask(task, nil)
	if len(errs) == 0 {
		t.Error("expected error for negative maxCostUsd")
	}
}

func TestValidateAgentTask_BudgetMaxCostInvalid(t *testing.T) {
	task := validTask()
	task.Spec.Budget = &corev1alpha1.BudgetSpec{MaxCostUsd: "abc"}
	errs := ValidateAgentTask(task, nil)
	if len(errs) == 0 {
		t.Error("expected error for invalid maxCostUsd")
	}
}

func TestValidateAgentTask_ValidBudget(t *testing.T) {
	task := validTask()
	task.Spec.Budget = &corev1alpha1.BudgetSpec{
		MaxTokens:  int64Ptr(1000),
		MaxCostUsd: "1.50",
	}
	errs := ValidateAgentTask(task, nil)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateAgentTask_ChildTierExceedsParent(t *testing.T) {
	parent := validTask()
	parent.Name = "parent"
	parent.Spec.Tier = "legionary"

	child := validTask()
	child.Spec.ParentTaskID = "parent"
	child.Spec.Tier = "centurion"

	errs := ValidateAgentTask(child, parent)
	found := false
	for _, e := range errs {
		if e.Field == "spec.tier" {
			found = true
		}
	}
	if !found {
		t.Error("expected tier escalation error")
	}
}

func TestValidateAgentTask_ChildTierEqualOK(t *testing.T) {
	parent := validTask()
	parent.Name = "parent"
	parent.Spec.Tier = "centurion"

	child := validTask()
	child.Spec.ParentTaskID = "parent"
	child.Spec.Tier = "centurion"

	errs := ValidateAgentTask(child, parent)
	for _, e := range errs {
		if e.Field == "spec.tier" {
			t.Errorf("unexpected tier error: %v", e)
		}
	}
}

func TestValidateAgentTask_ChildBudgetExceedsParent(t *testing.T) {
	parent := validTask()
	parent.Name = "parent"
	parent.Spec.Budget = &corev1alpha1.BudgetSpec{
		MaxTokens:  int64Ptr(1000),
		MaxCostUsd: "1.00",
	}

	child := validTask()
	child.Spec.ParentTaskID = "parent"
	child.Spec.Budget = &corev1alpha1.BudgetSpec{
		MaxTokens:  int64Ptr(2000),
		MaxCostUsd: "2.00",
	}

	errs := ValidateAgentTask(child, parent)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 budget errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateAgentTask_ChildCapabilityEscalation(t *testing.T) {
	parent := validTask()
	parent.Name = "parent"
	parent.Spec.Capabilities = []string{"shell"}

	child := validTask()
	child.Spec.ParentTaskID = "parent"
	child.Spec.Capabilities = []string{"shell", "network"}

	errs := ValidateAgentTask(child, parent)
	found := false
	for _, e := range errs {
		if e.Field == "spec.capabilities" {
			found = true
		}
	}
	if !found {
		t.Error("expected capability escalation error")
	}
}

func TestValidateAgentTask_ChildCapSubsetOK(t *testing.T) {
	parent := validTask()
	parent.Name = "parent"
	parent.Spec.Capabilities = []string{"shell", "network"}

	child := validTask()
	child.Spec.ParentTaskID = "parent"
	child.Spec.Capabilities = []string{"shell"}

	errs := ValidateAgentTask(child, parent)
	for _, e := range errs {
		if e.Field == "spec.capabilities" {
			t.Errorf("unexpected capability error: %v", e)
		}
	}
}

func TestValidateAgentTask_TribuneParentAutoInjectsSpawn(t *testing.T) {
	parent := validTask()
	parent.Name = "parent"
	parent.Spec.Tier = "tribune"
	parent.Spec.Capabilities = []string{"shell"}

	child := validTask()
	child.Spec.ParentTaskID = "parent"
	child.Spec.Capabilities = []string{"spawn"}

	errs := ValidateAgentTask(child, parent)
	for _, e := range errs {
		if e.Field == "spec.capabilities" {
			t.Errorf("unexpected capability error: spawn should be auto-injected for tribune parent: %v", e)
		}
	}
}

func TestValidateAgentTask_ValidNoParent(t *testing.T) {
	task := validTask()
	task.Spec.Timeout = intPtr(300)
	errs := ValidateAgentTask(task, nil)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestTierRank(t *testing.T) {
	if tierRank("legionary") >= tierRank("centurion") {
		t.Error("legionary should be lower than centurion")
	}
	if tierRank("centurion") >= tierRank("tribune") {
		t.Error("centurion should be lower than tribune")
	}
}
