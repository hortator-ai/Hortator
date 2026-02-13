/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func TestShouldRetry(t *testing.T) {
	r := &AgentTaskReconciler{}

	tests := []struct {
		name     string
		task     *corev1alpha1.AgentTask
		expected bool
	}{
		{
			name: "no retry spec",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: nil},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 0},
			},
			expected: false,
		},
		{
			name: "retry with maxAttempts=0",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{MaxAttempts: 0}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 0},
			},
			expected: false,
		},
		{
			name: "first attempt with maxAttempts=3",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{MaxAttempts: 3}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 0},
			},
			expected: true,
		},
		{
			name: "second attempt with maxAttempts=3",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{MaxAttempts: 3}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 2},
			},
			expected: true,
		},
		{
			name: "exhausted retries",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{MaxAttempts: 3}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 3},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.shouldRetry(tt.task)
			if got != tt.expected {
				t.Errorf("shouldRetry() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestComputeBackoff(t *testing.T) {
	r := &AgentTaskReconciler{}

	// computeBackoff adds ±25% jitter, so we check that results fall within
	// the expected range: [base*0.75, base*1.25] (clamped to min 1s).
	tests := []struct {
		name    string
		task    *corev1alpha1.AgentTask
		baseVal int // expected base value in seconds (before jitter)
	}{
		{
			name: "first attempt default backoff",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 30, MaxBackoffSeconds: 300}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 1},
			},
			baseVal: 30,
		},
		{
			name: "second attempt doubles",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 30, MaxBackoffSeconds: 300}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 2},
			},
			baseVal: 60,
		},
		{
			name: "third attempt quadruples",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 30, MaxBackoffSeconds: 300}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 3},
			},
			baseVal: 120,
		},
		{
			name: "capped at max",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 30, MaxBackoffSeconds: 60}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 5},
			},
			baseVal: 60,
		},
		{
			name: "custom base backoff",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 10, MaxBackoffSeconds: 300}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 1},
			},
			baseVal: 10,
		},
		{
			name: "nil retry spec uses defaults",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: nil},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 1},
			},
			baseVal: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to account for jitter randomness.
			// Jitter is ±25% but integer math can round up slightly, so we
			// use a generous 30% tolerance.
			for i := 0; i < 50; i++ {
				got := r.computeBackoff(tt.task)
				minVal := time.Duration(float64(tt.baseVal)*0.70) * time.Second
				if minVal < time.Second {
					minVal = time.Second
				}
				maxVal := time.Duration(float64(tt.baseVal)*1.30+1) * time.Second
				if got < minVal || got > maxVal {
					t.Errorf("computeBackoff() = %v, want between %v and %v (base=%ds, iteration %d)",
						got, minVal, maxVal, tt.baseVal, i)
				}
			}
		})
	}
}

func TestRecordAttempt(t *testing.T) {
	r := &AgentTaskReconciler{}
	now := metav1.Now()

	task := &corev1alpha1.AgentTask{
		Status: corev1alpha1.AgentTaskStatus{
			Attempts:  0,
			StartedAt: &now,
		},
	}

	exitCode := int32(7)
	r.recordAttempt(task, &exitCode, "entrypoint crash")

	if task.Status.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", task.Status.Attempts)
	}
	if len(task.Status.History) != 1 {
		t.Fatalf("History length = %d, want 1", len(task.Status.History))
	}

	record := task.Status.History[0]
	if record.Attempt != 1 {
		t.Errorf("record.Attempt = %d, want 1", record.Attempt)
	}
	if record.ExitCode == nil || *record.ExitCode != 7 {
		t.Errorf("record.ExitCode = %v, want 7", record.ExitCode)
	}
	if record.Reason != "entrypoint crash" {
		t.Errorf("record.Reason = %q, want %q", record.Reason, "entrypoint crash")
	}

	// Second attempt
	exitCode2 := int32(0)
	r.recordAttempt(task, &exitCode2, "completed")

	if task.Status.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", task.Status.Attempts)
	}
	if len(task.Status.History) != 2 {
		t.Fatalf("History length = %d, want 2", len(task.Status.History))
	}
}

func TestExtractTokenUsage(t *testing.T) {
	r := &AgentTaskReconciler{}

	tests := []struct {
		name       string
		output     string
		wantInput  int64
		wantOutput int64
		wantNil    bool
	}{
		{
			name:       "valid token line",
			output:     "[hortator-runtime] Done. Tokens: in=133 out=4096",
			wantInput:  133,
			wantOutput: 4096,
		},
		{
			name:       "multiline with token line",
			output:     "[hortator-runtime] Using Anthropic API...\n[hortator-runtime] Done. Tokens: in=38 out=54",
			wantInput:  38,
			wantOutput: 54,
		},
		{
			name:    "no token line",
			output:  "[hortator-runtime] Some other output",
			wantNil: true,
		},
		{
			name:    "empty output",
			output:  "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &corev1alpha1.AgentTask{
				Status: corev1alpha1.AgentTaskStatus{Output: tt.output},
			}
			r.extractTokenUsage(task)
			if tt.wantNil {
				if task.Status.TokensUsed != nil {
					t.Errorf("expected nil TokensUsed, got %+v", task.Status.TokensUsed)
				}
				return
			}
			if task.Status.TokensUsed == nil {
				t.Fatal("expected non-nil TokensUsed")
			}
			if task.Status.TokensUsed.Input != tt.wantInput {
				t.Errorf("Input = %d, want %d", task.Status.TokensUsed.Input, tt.wantInput)
			}
			if task.Status.TokensUsed.Output != tt.wantOutput {
				t.Errorf("Output = %d, want %d", task.Status.TokensUsed.Output, tt.wantOutput)
			}
		})
	}
}

func TestParseDurationString(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDurationString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDurationString(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseDurationString(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
