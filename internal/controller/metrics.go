package controller

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
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
)

var tracer = otel.Tracer("hortator.ai/operator")

func init() {
	metrics.Registry.MustRegister(tasksTotal, tasksActive, taskDuration)
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
func emitTaskEvent(ctx context.Context, eventName string, task *corev1alpha1.AgentTask) {
	_, span := tracer.Start(ctx, eventName)
	defer span.End()
	span.AddEvent(eventName, trace.WithAttributes(taskEventAttrs(task)...))
}
