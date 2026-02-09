package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

func intPtr(i int) *int { return &i }

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

	tests := []struct {
		name     string
		task     *corev1alpha1.AgentTask
		expected time.Duration
	}{
		{
			name: "first attempt default backoff",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 30, MaxBackoffSeconds: 300}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 1},
			},
			expected: 30 * time.Second,
		},
		{
			name: "second attempt doubles",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 30, MaxBackoffSeconds: 300}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 2},
			},
			expected: 60 * time.Second,
		},
		{
			name: "third attempt quadruples",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 30, MaxBackoffSeconds: 300}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 3},
			},
			expected: 120 * time.Second,
		},
		{
			name: "capped at max",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 30, MaxBackoffSeconds: 60}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 5},
			},
			expected: 60 * time.Second,
		},
		{
			name: "custom base backoff",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: &corev1alpha1.RetrySpec{BackoffSeconds: 10, MaxBackoffSeconds: 300}},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 1},
			},
			expected: 10 * time.Second,
		},
		{
			name: "nil retry spec uses defaults",
			task: &corev1alpha1.AgentTask{
				Spec:   corev1alpha1.AgentTaskSpec{Retry: nil},
				Status: corev1alpha1.AgentTaskStatus{Attempts: 1},
			},
			expected: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.computeBackoff(tt.task)
			if got != tt.expected {
				t.Errorf("computeBackoff() = %v, want %v", got, tt.expected)
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
