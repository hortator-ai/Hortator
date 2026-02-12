/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

// Prometheus metrics
var (
	tasksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hortator_tasks_total",
			Help: "Total number of AgentTasks by phase and namespace",
		},
		[]string{"phase", "namespace"},
	)
	tasksActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hortator_tasks_active",
			Help: "Number of currently active (Running) AgentTasks by namespace",
		},
		[]string{"namespace"},
	)
	taskDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hortator_task_duration_seconds",
			Help:    "Duration of completed AgentTasks in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1s to ~16384s
		},
	)
	taskCostUsd = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hortator_task_cost_usd",
			Help:    "Estimated cost in USD per completed AgentTask",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 25.0},
		},
	)
	budgetExceededTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hortator_budget_exceeded_total",
			Help: "Total number of AgentTasks that exceeded their budget",
		},
		[]string{"namespace"},
	)
	stuckDetectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hortator_stuck_detected_total",
			Help: "Total number of stuck agent detections by action and namespace",
		},
		[]string{"action", "namespace"},
	)
	taskToolDiversity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hortator_task_tool_diversity",
			Help: "Tool diversity score (unique/total) for running tasks",
		},
		[]string{"namespace", "task"},
	)
)

var tracer = otel.Tracer("hortator.ai/operator")

func init() {
	metrics.Registry.MustRegister(
		tasksTotal, tasksActive, taskDuration,
		taskCostUsd, budgetExceededTotal,
		stuckDetectedTotal, taskToolDiversity,
	)
}

func taskEventAttrs(task *corev1alpha1.AgentTask) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("hortator.task.id", task.Name),
		attribute.String("hortator.task.namespace", task.Namespace),
		attribute.String("hortator.task.phase", string(task.Status.Phase)),
		attribute.String("hortator.task.role", task.Spec.Role),
		attribute.String("hortator.task.tier", task.Spec.Tier),
		attribute.String("hortator.task.parent", task.Spec.ParentTaskID),
	}
}

// emitTaskEvent starts a span and records a named event with task attributes.
// Optional extra attributes are appended (used for terminal events with token/cost data).
func emitTaskEvent(ctx context.Context, eventName string, task *corev1alpha1.AgentTask, extra ...attribute.KeyValue) {
	attrs := taskEventAttrs(task)
	attrs = append(attrs, extra...)
	_, span := tracer.Start(ctx, eventName)
	defer span.End()
	span.AddEvent(eventName, trace.WithAttributes(attrs...))
}

// terminalEventAttrs returns extra OTel attributes for completed/failed events,
// including token usage, cost, duration, and attempt count.
func terminalEventAttrs(task *corev1alpha1.AgentTask) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("hortator.task.duration", task.Status.Duration),
		attribute.Int("hortator.task.attempts", task.Status.Attempts),
	}
	if task.Status.TokensUsed != nil {
		attrs = append(attrs,
			attribute.Int64("hortator.task.tokens.input", task.Status.TokensUsed.Input),
			attribute.Int64("hortator.task.tokens.output", task.Status.TokensUsed.Output),
		)
	}
	if task.Status.EstimatedCostUsd != "" {
		attrs = append(attrs, attribute.String("hortator.task.cost_usd", task.Status.EstimatedCostUsd))
	}
	return attrs
}
