/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	return s
}

func TestEnforcePolicy(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()

	int64Ptr := func(v int64) *int64 { return &v }
	intPtr := func(v int) *int { return &v }

	tests := []struct {
		name     string
		policies []corev1alpha1.AgentPolicy
		task     *corev1alpha1.AgentTask
		wantPass bool
	}{
		{
			name:     "no policies passes",
			policies: nil,
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Capabilities: []string{"shell"}},
			},
			wantPass: true,
		},
		{
			name: "denied capability blocks",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{DeniedCapabilities: []string{"shell"}},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Capabilities: []string{"web-fetch", "shell"}},
			},
			wantPass: false,
		},
		{
			name: "allowed capability passes",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{AllowedCapabilities: []string{"shell", "web-fetch"}},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Capabilities: []string{"shell"}},
			},
			wantPass: true,
		},
		{
			name: "capability not in allowlist blocks",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{AllowedCapabilities: []string{"web-fetch"}},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Capabilities: []string{"shell"}},
			},
			wantPass: false,
		},
		{
			name: "max budget tokens exceeded blocks",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{MaxBudget: &corev1alpha1.BudgetSpec{MaxTokens: int64Ptr(1000)}},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Budget: &corev1alpha1.BudgetSpec{MaxTokens: int64Ptr(5000)}},
			},
			wantPass: false,
		},
		{
			name: "max budget cost exceeded blocks",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{MaxBudget: &corev1alpha1.BudgetSpec{MaxCostUsd: "0.50"}},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Budget: &corev1alpha1.BudgetSpec{MaxCostUsd: "1.00"}},
			},
			wantPass: false,
		},
		{
			name: "max timeout exceeded blocks",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{MaxTimeout: intPtr(300)},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Timeout: intPtr(600)},
			},
			wantPass: false,
		},
		{
			name: "max tier exceeded blocks",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{MaxTier: "centurion"},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Tier: "tribune"},
			},
			wantPass: false,
		},
		{
			name: "tier within limit passes",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{MaxTier: "centurion"},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Tier: "legionary"},
			},
			wantPass: true,
		},
		{
			name: "max concurrent tasks blocks when at limit",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{MaxConcurrentTasks: intPtr(1)},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t-new", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test"},
			},
			wantPass: false,
		},
		{
			name: "allowed images with glob pattern passes",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{AllowedImages: []string{"ghcr.io/hortator-ai/*"}},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Image: "ghcr.io/hortator-ai/hortator/agent:latest"},
			},
			wantPass: true,
		},
		{
			name: "allowed images blocks non-matching",
			policies: []corev1alpha1.AgentPolicy{{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec:       corev1alpha1.AgentPolicySpec{AllowedImages: []string{"ghcr.io/hortator-ai/*"}},
			}},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Image: "docker.io/evil/agent:latest"},
			},
			wantPass: false,
		},
		{
			name: "multiple policies all must pass - second blocks",
			policies: []corev1alpha1.AgentPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
					Spec:       corev1alpha1.AgentPolicySpec{AllowedCapabilities: []string{"shell", "web-fetch"}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "default"},
					Spec:       corev1alpha1.AgentPolicySpec{MaxTier: "legionary"},
				},
			},
			task: &corev1alpha1.AgentTask{
				ObjectMeta: metav1.ObjectMeta{Name: "t1", Namespace: "default"},
				Spec:       corev1alpha1.AgentTaskSpec{Prompt: "test", Capabilities: []string{"shell"}, Tier: "tribune"},
			},
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{}
			for i := range tt.policies {
				objs = append(objs, &tt.policies[i])
			}

			// For the concurrent tasks test, add a running task
			if tt.name == "max concurrent tasks blocks when at limit" {
				runningTask := &corev1alpha1.AgentTask{
					ObjectMeta: metav1.ObjectMeta{Name: "t-running", Namespace: "default"},
					Spec:       corev1alpha1.AgentTaskSpec{Prompt: "running"},
					Status:     corev1alpha1.AgentTaskStatus{Phase: corev1alpha1.AgentTaskPhaseRunning},
				}
				objs = append(objs, runningTask)
			}

			fc := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
			r := &AgentTaskReconciler{
				Client: fc,
				Scheme: scheme,
				defaults: ClusterDefaults{
					DefaultImage: "ghcr.io/hortator-ai/hortator/agent:latest",
				},
			}

			violation := r.enforcePolicy(ctx, tt.task)
			if tt.wantPass && violation != "" {
				t.Errorf("expected pass, got violation: %s", violation)
			}
			if !tt.wantPass && violation == "" {
				t.Error("expected violation, got pass")
			}
		})
	}
}
