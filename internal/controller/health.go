/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

// StuckScore holds the results of behavioral stuck-detection analysis.
type StuckScore struct {
	ToolDiversity    float64 // unique_tools / total_tool_calls (0-1)
	RepeatedPrompts  int     // count of identical prompt hashes in window
	StatusStaleMins  float64 // minutes since last progress report
	Aggregate        float64 // weighted overall score (0-1, higher = more stuck)
	IsStuck          bool
	Reason           string
}

var (
	reToolCall   = regexp.MustCompile(`\[hortator-agentic\] Tool call: (\w+)\(`)
	rePromptHash = regexp.MustCompile(`\[hortator-agentic\] Prompt hash: ([a-f0-9]+)`)
)

// checkStuckSignals analyses the running task's pod logs for behavioral signals
// that indicate the agent may be stuck (looping, repetitive, or stalled).
func (r *AgentTaskReconciler) checkStuckSignals(ctx context.Context,
	task *corev1alpha1.AgentTask, pod *corev1.Pod, cfg StuckDetectionConfig) StuckScore {

	score := StuckScore{}

	// Collect recent logs from the agent container
	logs := r.collectPodLogs(ctx, task.Namespace, pod.Name)
	if logs == "" {
		return score
	}

	// ── Signal 1: Tool diversity ──────────────────────────────────────────
	// Lower diversity = more repetitive behavior = likely stuck.
	toolMatches := reToolCall.FindAllStringSubmatch(logs, -1)
	if len(toolMatches) > 2 {
		toolSet := make(map[string]bool)
		for _, m := range toolMatches {
			toolSet[m[1]] = true
		}
		score.ToolDiversity = float64(len(toolSet)) / float64(len(toolMatches))
	} else {
		score.ToolDiversity = 1.0 // Not enough data, assume healthy
	}

	// ── Signal 2: Prompt repetition ───────────────────────────────────────
	// Same prompt hash appearing multiple times = going in circles.
	hashMatches := rePromptHash.FindAllStringSubmatch(logs, -1)
	if len(hashMatches) > 1 {
		hashCounts := make(map[string]int)
		for _, m := range hashMatches {
			hashCounts[m[1]]++
		}
		maxCount := 0
		for _, count := range hashCounts {
			if count > maxCount {
				maxCount = count
			}
		}
		score.RepeatedPrompts = maxCount
	}

	// ── Signal 3: Status staleness ────────────────────────────────────────
	// Check how long since the agent last reported progress.
	if ann, ok := task.Annotations["hortator.ai/last-progress"]; ok && ann != "" {
		if t, err := time.Parse(time.RFC3339, ann); err == nil {
			score.StatusStaleMins = time.Since(t).Minutes()
		}
	} else if task.Status.StartedAt != nil {
		// If no progress annotation exists, use task start time
		score.StatusStaleMins = time.Since(task.Status.StartedAt.Time).Minutes()
	}

	// ── Aggregate score ───────────────────────────────────────────────────
	// Weighted combination: tool diversity (40%), prompt repetition (35%), staleness (25%)
	diversityPenalty := 0.0
	if score.ToolDiversity < cfg.ToolDiversityMin {
		diversityPenalty = (cfg.ToolDiversityMin - score.ToolDiversity) / cfg.ToolDiversityMin
	}

	repetitionPenalty := 0.0
	if cfg.MaxRepeatedPrompts > 0 && score.RepeatedPrompts > cfg.MaxRepeatedPrompts {
		repetitionPenalty = float64(score.RepeatedPrompts-cfg.MaxRepeatedPrompts) /
			float64(cfg.MaxRepeatedPrompts)
		if repetitionPenalty > 1.0 {
			repetitionPenalty = 1.0
		}
	}

	stalenessPenalty := 0.0
	if cfg.StatusStaleMinutes > 0 && score.StatusStaleMins > float64(cfg.StatusStaleMinutes) {
		stalenessPenalty = (score.StatusStaleMins - float64(cfg.StatusStaleMinutes)) /
			float64(cfg.StatusStaleMinutes)
		if stalenessPenalty > 1.0 {
			stalenessPenalty = 1.0
		}
	}

	score.Aggregate = 0.40*diversityPenalty + 0.35*repetitionPenalty + 0.25*stalenessPenalty

	// Determine if stuck (threshold: 0.5 aggregate score)
	if score.Aggregate >= 0.5 {
		score.IsStuck = true
		reasons := []string{}
		if diversityPenalty > 0 {
			reasons = append(reasons, fmt.Sprintf("low tool diversity (%.2f < %.2f)", score.ToolDiversity, cfg.ToolDiversityMin))
		}
		if repetitionPenalty > 0 {
			reasons = append(reasons, fmt.Sprintf("repeated prompts (%d > %d)", score.RepeatedPrompts, cfg.MaxRepeatedPrompts))
		}
		if stalenessPenalty > 0 {
			reasons = append(reasons, fmt.Sprintf("stale progress (%.0fm > %dm)", score.StatusStaleMins, cfg.StatusStaleMinutes))
		}
		score.Reason = strings.Join(reasons, "; ")
	}

	return score
}

// executeStuckAction performs the configured action for a stuck agent.
func (r *AgentTaskReconciler) executeStuckAction(ctx context.Context,
	task *corev1alpha1.AgentTask, pod *corev1.Pod, score StuckScore, action string) error {

	logger := log.FromContext(ctx)

	// Update metric
	stuckDetectedTotal.WithLabelValues(action, task.Namespace).Inc()

	switch action {
	case "warn":
		// Emit event and OTel span, but don't kill the agent
		emitTaskEvent(ctx, "hortator.health.stuck_detected", task)
		r.Recorder.Eventf(task, corev1.EventTypeWarning, "StuckDetected",
			"Agent may be stuck (score=%.2f): %s", score.Aggregate, score.Reason)
		logger.Info("Stuck agent detected (warn)", "task", task.Name,
			"score", score.Aggregate, "reason", score.Reason)

	case "kill":
		// Kill the pod and fail the task
		emitTaskEvent(ctx, "hortator.health.stuck_killed", task)
		r.Recorder.Eventf(task, corev1.EventTypeWarning, "StuckKilled",
			"Killed stuck agent (score=%.2f): %s", score.Aggregate, score.Reason)
		logger.Info("Killing stuck agent", "task", task.Name,
			"score", score.Aggregate, "reason", score.Reason)

		if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
			return err
		}

		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = fmt.Sprintf("Killed: agent stuck (score=%.2f): %s", score.Aggregate, score.Reason)
		setCompletionStatus(task)
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		tasksActive.WithLabelValues(task.Namespace).Dec()
		if err := r.Status().Update(ctx, task); err != nil {
			return err
		}
		r.notifyParentTask(ctx, task)

	case "escalate":
		// Kill and notify parent with a stuck report
		emitTaskEvent(ctx, "hortator.health.stuck_escalated", task)
		r.Recorder.Eventf(task, corev1.EventTypeWarning, "StuckEscalated",
			"Escalating stuck agent (score=%.2f): %s", score.Aggregate, score.Reason)
		logger.Info("Escalating stuck agent", "task", task.Name,
			"score", score.Aggregate, "reason", score.Reason)

		if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
			return err
		}

		task.Status.Phase = corev1alpha1.AgentTaskPhaseFailed
		task.Status.Message = fmt.Sprintf("Escalated: agent stuck (score=%.2f): %s", score.Aggregate, score.Reason)
		setCompletionStatus(task)
		tasksTotal.WithLabelValues(string(corev1alpha1.AgentTaskPhaseFailed), task.Namespace).Inc()
		tasksActive.WithLabelValues(task.Namespace).Dec()
		if err := r.Status().Update(ctx, task); err != nil {
			return err
		}
		r.notifyParentTask(ctx, task)
	}

	return nil
}

// resolveStuckConfig merges the cluster-level stuck detection config with
// per-task overrides from the AgentTask's health spec.
func resolveStuckConfig(defaults StuckDetectionConfig, task *corev1alpha1.AgentTask) StuckDetectionConfig {
	cfg := defaults

	if task.Spec.Health == nil || task.Spec.Health.StuckDetection == nil {
		return cfg
	}

	override := task.Spec.Health.StuckDetection
	if override.ToolDiversityMin != nil {
		cfg.ToolDiversityMin = *override.ToolDiversityMin
	}
	if override.MaxRepeatedPrompts != nil {
		cfg.MaxRepeatedPrompts = *override.MaxRepeatedPrompts
	}
	if override.StatusStaleMinutes != nil {
		cfg.StatusStaleMinutes = *override.StatusStaleMinutes
	}
	if override.Action != "" {
		cfg.Action = override.Action
	}

	return cfg
}
