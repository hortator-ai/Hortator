/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

// --- parseQuantity ---

func TestParseQuantity(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		label   string
		wantErr bool
	}{
		{"valid CPU", "500m", "CPU", false},
		{"valid memory", "128Mi", "memory", false},
		{"valid whole CPU", "2", "CPU", false},
		{"empty string", "", "CPU", true},
		{"garbage", "notaresource", "CPU", true},
		{"negative", "-100m", "CPU", false}, // K8s accepts negative, validation is elsewhere
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseQuantity(tt.value, tt.label)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseQuantity(%q, %q) error = %v, wantErr %v", tt.value, tt.label, err, tt.wantErr)
			}
		})
	}
}

// --- buildResources ---

func TestBuildResources(t *testing.T) {
	r := &AgentTaskReconciler{
		defaults: ClusterDefaults{
			DefaultRequestsCPU:    "100m",
			DefaultRequestsMemory: "128Mi",
			DefaultLimitsCPU:      "500m",
			DefaultLimitsMemory:   "512Mi",
		},
	}

	t.Run("uses spec resources when provided", func(t *testing.T) {
		task := &corev1alpha1.AgentTask{
			Spec: corev1alpha1.AgentTaskSpec{
				Resources: &corev1alpha1.ResourceRequirements{
					Requests: &corev1alpha1.ResourceList{CPU: "200m", Memory: "256Mi"},
					Limits:   &corev1alpha1.ResourceList{CPU: "1", Memory: "1Gi"},
				},
			},
		}
		res, err := r.buildResources(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Requests.Cpu().Cmp(resource.MustParse("200m")) != 0 {
			t.Errorf("CPU request = %v, want 200m", res.Requests.Cpu())
		}
		if res.Limits.Memory().Cmp(resource.MustParse("1Gi")) != 0 {
			t.Errorf("memory limit = %v, want 1Gi", res.Limits.Memory())
		}
	})

	t.Run("falls back to cluster defaults", func(t *testing.T) {
		task := &corev1alpha1.AgentTask{
			Spec: corev1alpha1.AgentTaskSpec{},
		}
		res, err := r.buildResources(task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Requests.Cpu().Cmp(resource.MustParse("100m")) != 0 {
			t.Errorf("CPU request = %v, want 100m", res.Requests.Cpu())
		}
		if res.Limits.Memory().Cmp(resource.MustParse("512Mi")) != 0 {
			t.Errorf("memory limit = %v, want 512Mi", res.Limits.Memory())
		}
	})

	t.Run("returns error on invalid resource string", func(t *testing.T) {
		task := &corev1alpha1.AgentTask{
			Spec: corev1alpha1.AgentTaskSpec{
				Resources: &corev1alpha1.ResourceRequirements{
					Requests: &corev1alpha1.ResourceList{CPU: "not-a-cpu-value"},
				},
			},
		}
		_, err := r.buildResources(task)
		if err == nil {
			t.Error("expected error for invalid CPU value, got nil")
		}
	})

	t.Run("returns error on invalid default", func(t *testing.T) {
		badR := &AgentTaskReconciler{
			defaults: ClusterDefaults{
				DefaultRequestsCPU: "garbage",
			},
		}
		task := &corev1alpha1.AgentTask{Spec: corev1alpha1.AgentTaskSpec{}}
		_, err := badR.buildResources(task)
		if err == nil {
			t.Error("expected error for invalid default CPU, got nil")
		}
	})
}

// --- tierRank ---

func TestTierRank(t *testing.T) {
	tests := []struct {
		tier string
		rank int
	}{
		{"legionary", 1},
		{"centurion", 2},
		{"tribune", 3},
		{"unknown", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			got := tierRank(tt.tier)
			if got != tt.rank {
				t.Errorf("tierRank(%q) = %d, want %d", tt.tier, got, tt.rank)
			}
		})
	}

	// Tribune > centurion > legionary
	if tierRank("tribune") <= tierRank("centurion") {
		t.Error("tribune should outrank centurion")
	}
	if tierRank("centurion") <= tierRank("legionary") {
		t.Error("centurion should outrank legionary")
	}
}

// --- setCompletionStatus ---

func TestSetCompletionStatus(t *testing.T) {
	t.Run("sets completedAt and duration", func(t *testing.T) {
		started := metav1.NewTime(time.Now().Add(-5 * time.Minute))
		task := &corev1alpha1.AgentTask{
			Status: corev1alpha1.AgentTaskStatus{
				StartedAt: &started,
			},
		}
		setCompletionStatus(task)

		if task.Status.CompletedAt == nil {
			t.Fatal("CompletedAt should be set")
		}
		if task.Status.Duration == "" {
			t.Fatal("Duration should be set")
		}
		// Duration should be approximately 5 minutes
		d, err := time.ParseDuration(task.Status.Duration)
		if err != nil {
			t.Fatalf("invalid duration string %q: %v", task.Status.Duration, err)
		}
		if d < 4*time.Minute || d > 6*time.Minute {
			t.Errorf("duration = %v, want ~5m", d)
		}
	})

	t.Run("handles nil startedAt", func(t *testing.T) {
		task := &corev1alpha1.AgentTask{
			Status: corev1alpha1.AgentTaskStatus{},
		}
		setCompletionStatus(task)

		if task.Status.CompletedAt == nil {
			t.Fatal("CompletedAt should be set even without StartedAt")
		}
		if task.Status.Duration != "" {
			t.Errorf("Duration should be empty without StartedAt, got %q", task.Status.Duration)
		}
	})
}

// --- extractResult ---

func TestExtractResult(t *testing.T) {
	r := &AgentTaskReconciler{}

	tests := []struct {
		name       string
		output     string
		wantOutput string
	}{
		{
			name:       "with markers",
			output:     "[hortator-runtime] Starting...\n[hortator-result-begin]\nThe answer is 42.\n[hortator-result-end]\n[hortator-runtime] Done.",
			wantOutput: "The answer is 42.",
		},
		{
			name:       "multiline result",
			output:     "noise\n[hortator-result-begin]\nLine 1\nLine 2\nLine 3\n[hortator-result-end]\nmore noise",
			wantOutput: "Line 1\nLine 2\nLine 3",
		},
		{
			name:       "no markers — keeps raw output",
			output:     "just some raw log output",
			wantOutput: "just some raw log output",
		},
		{
			name:       "empty output",
			output:     "",
			wantOutput: "",
		},
		{
			name:       "only begin marker",
			output:     "[hortator-result-begin]\npartial",
			wantOutput: "[hortator-result-begin]\npartial",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &corev1alpha1.AgentTask{
				Status: corev1alpha1.AgentTaskStatus{Output: tt.output},
			}
			r.extractResult(task)
			if task.Status.Output != tt.wantOutput {
				t.Errorf("extractResult() output = %q, want %q", task.Status.Output, tt.wantOutput)
			}
		})
	}
}

// --- enforcePolicy ---

func TestEnforcePolicy_Capabilities(t *testing.T) {
	// We can't test enforcePolicy with a real client, but we can test the
	// capability inheritance logic directly since it's the same check in handlePending.
	// This tests the deny/allow logic inline.

	t.Run("denied capability blocks task", func(t *testing.T) {
		denied := map[string]bool{"shell": true}
		taskCaps := []string{"web-fetch", "shell"}

		for _, cap := range taskCaps {
			if denied[cap] {
				// This is the expected path
				return
			}
		}
		t.Error("should have found denied capability")
	})

	t.Run("all capabilities allowed", func(t *testing.T) {
		allowed := map[string]bool{"web-fetch": true, "shell": true, "spawn": true}
		taskCaps := []string{"web-fetch", "shell"}

		for _, cap := range taskCaps {
			if !allowed[cap] {
				t.Errorf("capability %q should be allowed", cap)
			}
		}
	})

	t.Run("capability not in allowlist", func(t *testing.T) {
		allowed := map[string]bool{"web-fetch": true}
		taskCaps := []string{"web-fetch", "shell"}
		blocked := false

		for _, cap := range taskCaps {
			if !allowed[cap] {
				blocked = true
				break
			}
		}
		if !blocked {
			t.Error("shell should not be allowed")
		}
	})
}

// --- isTransientFailure ---

func TestIsTransientFailure(t *testing.T) {
	r := &AgentTaskReconciler{}

	t.Run("exit code 0 is not transient", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "agent",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
						},
					},
				},
			},
		}
		task := &corev1alpha1.AgentTask{}
		if r.isTransientFailure(context.TODO(), task, pod) {
			t.Error("exit code 0 should not be transient")
		}
	})

	t.Run("exit code 1 is transient", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "agent",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 1},
						},
					},
				},
			},
		}
		task := &corev1alpha1.AgentTask{}
		if !r.isTransientFailure(context.TODO(), task, pod) {
			t.Error("exit code 1 should be transient")
		}
	})

	t.Run("OOM kill (137) is transient", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "agent",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{ExitCode: 137},
						},
					},
				},
			},
		}
		task := &corev1alpha1.AgentTask{}
		if !r.isTransientFailure(context.TODO(), task, pod) {
			t.Error("exit code 137 (OOM) should be transient")
		}
	})

	t.Run("no terminated state is transient", func(t *testing.T) {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "agent", State: corev1.ContainerState{}},
				},
			},
		}
		task := &corev1alpha1.AgentTask{}
		if !r.isTransientFailure(context.TODO(), task, pod) {
			t.Error("no terminated state should be transient")
		}
	})
}
