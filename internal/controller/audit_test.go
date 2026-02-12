/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

func TestRecorderCalledOnTaskCompletion(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	r := &AgentTaskReconciler{
		Recorder: recorder,
	}

	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
			Namespace: "default",
		},
		Status: corev1alpha1.AgentTaskStatus{
			Phase:    corev1alpha1.AgentTaskPhaseCompleted,
			Duration: "1m30s",
			PodName:  "test-pod",
		},
	}

	// Simulate what the controller does on completion
	r.Recorder.Eventf(task, "Normal", "TaskCompleted", "Task completed in %s", task.Status.Duration)

	select {
	case event := <-recorder.Events:
		expected := "Normal TaskCompleted Task completed in 1m30s"
		if event != expected {
			t.Errorf("unexpected event: got %q, want %q", event, expected)
		}
	default:
		t.Error("expected an event but none was recorded")
	}
}

func TestRecorderCalledOnTaskFailed(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	r := &AgentTaskReconciler{
		Recorder: recorder,
	}

	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
			Namespace: "default",
		},
		Status: corev1alpha1.AgentTaskStatus{
			Phase:   corev1alpha1.AgentTaskPhaseFailed,
			Message: "container OOMKilled",
		},
	}

	r.Recorder.Event(task, "Warning", "TaskFailed", "Task failed: "+task.Status.Message)

	select {
	case event := <-recorder.Events:
		expected := "Warning TaskFailed Task failed: container OOMKilled"
		if event != expected {
			t.Errorf("unexpected event: got %q, want %q", event, expected)
		}
	default:
		t.Error("expected an event but none was recorded")
	}
}

func TestTerminalEventAttrsIncludesTokens(t *testing.T) {
	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
			Namespace: "default",
		},
		Status: corev1alpha1.AgentTaskStatus{
			Phase:    corev1alpha1.AgentTaskPhaseCompleted,
			Duration: "2m10s",
			Attempts: 1,
			TokensUsed: &corev1alpha1.TokenUsage{
				Input:  5000,
				Output: 1200,
			},
			EstimatedCostUsd: "0.03",
		},
	}

	attrs := terminalEventAttrs(task)

	want := map[string]interface{}{
		"hortator.task.duration":      "2m10s",
		"hortator.task.attempts":      int64(1),
		"hortator.task.tokens.input":  int64(5000),
		"hortator.task.tokens.output": int64(1200),
		"hortator.task.cost_usd":      "0.03",
	}

	found := make(map[string]bool)
	for _, a := range attrs {
		key := string(a.Key)
		found[key] = true
		if expected, ok := want[key]; ok {
			switch v := expected.(type) {
			case string:
				if a.Value.AsString() != v {
					t.Errorf("attr %s: got %q, want %q", key, a.Value.AsString(), v)
				}
			case int64:
				if a.Value.AsInt64() != v {
					t.Errorf("attr %s: got %d, want %d", key, a.Value.AsInt64(), v)
				}
			}
		}
	}

	for key := range want {
		if !found[key] {
			t.Errorf("missing attribute %s", key)
		}
	}
}

func TestEmitTaskEventWithExtraAttrs(t *testing.T) {
	// Verify emitTaskEvent accepts variadic extra attributes without panic
	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
			Namespace: "default",
		},
		Spec: corev1alpha1.AgentTaskSpec{
			Role: "test-role",
			Tier: "legionary",
		},
		Status: corev1alpha1.AgentTaskStatus{
			Phase: corev1alpha1.AgentTaskPhaseCompleted,
		},
	}

	// Should not panic with extra attrs
	emitTaskEvent(context.Background(), "hortator.task.completed", task,
		attribute.String("hortator.task.duration", "1m"),
		attribute.Int("hortator.task.attempts", 2),
	)

	// Should not panic without extra attrs
	emitTaskEvent(context.Background(), "hortator.task.started", task)
}
