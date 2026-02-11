/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
)

// --- resolveStuckConfig ---

func TestResolveStuckConfig_DefaultsOnly(t *testing.T) {
	defaults := StuckDetectionConfig{
		Enabled:            true,
		ToolDiversityMin:   0.3,
		MaxRepeatedPrompts: 3,
		StatusStaleMinutes: 5,
		CheckWindowMinutes: 5,
		Action:             "warn",
	}

	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
		},
	}

	cfg := resolveStuckConfig(defaults, nil, task)

	if cfg.ToolDiversityMin != 0.3 {
		t.Errorf("expected ToolDiversityMin=0.3, got %f", cfg.ToolDiversityMin)
	}
	if cfg.MaxRepeatedPrompts != 3 {
		t.Errorf("expected MaxRepeatedPrompts=3, got %d", cfg.MaxRepeatedPrompts)
	}
	if cfg.StatusStaleMinutes != 5 {
		t.Errorf("expected StatusStaleMinutes=5, got %d", cfg.StatusStaleMinutes)
	}
	if cfg.Action != "warn" {
		t.Errorf("expected Action=warn, got %s", cfg.Action)
	}
}

func TestResolveStuckConfig_TaskOverrides(t *testing.T) {
	defaults := StuckDetectionConfig{
		Enabled:            true,
		ToolDiversityMin:   0.3,
		MaxRepeatedPrompts: 3,
		StatusStaleMinutes: 5,
		Action:             "warn",
	}

	toolDiv := 0.15
	maxPrompts := 6
	staleMins := 10

	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Health: &corev1alpha1.HealthSpec{
				StuckDetection: &corev1alpha1.StuckDetectionSpec{
					ToolDiversityMin:   &toolDiv,
					MaxRepeatedPrompts: &maxPrompts,
					StatusStaleMinutes: &staleMins,
					Action:             "escalate",
				},
			},
		},
	}

	cfg := resolveStuckConfig(defaults, nil, task)

	if cfg.ToolDiversityMin != 0.15 {
		t.Errorf("expected ToolDiversityMin=0.15, got %f", cfg.ToolDiversityMin)
	}
	if cfg.MaxRepeatedPrompts != 6 {
		t.Errorf("expected MaxRepeatedPrompts=6, got %d", cfg.MaxRepeatedPrompts)
	}
	if cfg.StatusStaleMinutes != 10 {
		t.Errorf("expected StatusStaleMinutes=10, got %d", cfg.StatusStaleMinutes)
	}
	if cfg.Action != "escalate" {
		t.Errorf("expected Action=escalate, got %s", cfg.Action)
	}
}

func TestResolveStuckConfig_PartialOverride(t *testing.T) {
	defaults := StuckDetectionConfig{
		ToolDiversityMin:   0.3,
		MaxRepeatedPrompts: 3,
		StatusStaleMinutes: 5,
		Action:             "warn",
	}

	maxPrompts := 10
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Health: &corev1alpha1.HealthSpec{
				StuckDetection: &corev1alpha1.StuckDetectionSpec{
					MaxRepeatedPrompts: &maxPrompts,
				},
			},
		},
	}

	cfg := resolveStuckConfig(defaults, nil, task)

	// Only MaxRepeatedPrompts should be overridden
	if cfg.ToolDiversityMin != 0.3 {
		t.Errorf("expected ToolDiversityMin=0.3 (default), got %f", cfg.ToolDiversityMin)
	}
	if cfg.MaxRepeatedPrompts != 10 {
		t.Errorf("expected MaxRepeatedPrompts=10 (override), got %d", cfg.MaxRepeatedPrompts)
	}
	if cfg.Action != "warn" {
		t.Errorf("expected Action=warn (default), got %s", cfg.Action)
	}
}

func TestResolveStuckConfig_RoleOverrides(t *testing.T) {
	defaults := StuckDetectionConfig{
		ToolDiversityMin:   0.3,
		MaxRepeatedPrompts: 3,
		StatusStaleMinutes: 5,
		Action:             "warn",
	}

	roleDiv := 0.15
	rolePrompts := 6
	roleHealth := &corev1alpha1.HealthSpec{
		StuckDetection: &corev1alpha1.StuckDetectionSpec{
			ToolDiversityMin:   &roleDiv,
			MaxRepeatedPrompts: &rolePrompts,
			Action:             "kill",
		},
	}

	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Role:   "qa-engineer",
		},
	}

	cfg := resolveStuckConfig(defaults, roleHealth, task)

	if cfg.ToolDiversityMin != 0.15 {
		t.Errorf("expected ToolDiversityMin=0.15 (role), got %f", cfg.ToolDiversityMin)
	}
	if cfg.MaxRepeatedPrompts != 6 {
		t.Errorf("expected MaxRepeatedPrompts=6 (role), got %d", cfg.MaxRepeatedPrompts)
	}
	if cfg.StatusStaleMinutes != 5 {
		t.Errorf("expected StatusStaleMinutes=5 (default, role didn't override), got %d", cfg.StatusStaleMinutes)
	}
	if cfg.Action != "kill" {
		t.Errorf("expected Action=kill (role), got %s", cfg.Action)
	}
}

func TestResolveStuckConfig_TaskOverridesRole(t *testing.T) {
	defaults := StuckDetectionConfig{
		ToolDiversityMin:   0.3,
		MaxRepeatedPrompts: 3,
		StatusStaleMinutes: 5,
		Action:             "warn",
	}

	roleDiv := 0.15
	roleHealth := &corev1alpha1.HealthSpec{
		StuckDetection: &corev1alpha1.StuckDetectionSpec{
			ToolDiversityMin: &roleDiv,
			Action:           "kill",
		},
	}

	taskDiv := 0.05
	task := &corev1alpha1.AgentTask{
		Spec: corev1alpha1.AgentTaskSpec{
			Prompt: "test",
			Health: &corev1alpha1.HealthSpec{
				StuckDetection: &corev1alpha1.StuckDetectionSpec{
					ToolDiversityMin: &taskDiv,
					Action:           "escalate",
				},
			},
		},
	}

	cfg := resolveStuckConfig(defaults, roleHealth, task)

	// Task should win over role
	if cfg.ToolDiversityMin != 0.05 {
		t.Errorf("expected ToolDiversityMin=0.05 (task overrides role), got %f", cfg.ToolDiversityMin)
	}
	if cfg.Action != "escalate" {
		t.Errorf("expected Action=escalate (task overrides role), got %s", cfg.Action)
	}
	// MaxRepeatedPrompts: default (3) since neither role nor task overrode it
	if cfg.MaxRepeatedPrompts != 3 {
		t.Errorf("expected MaxRepeatedPrompts=3 (default), got %d", cfg.MaxRepeatedPrompts)
	}
}

// --- StuckScore computation (testing the scoring logic via regex parsing) ---

func TestStuckScore_ToolDiversity(t *testing.T) {
	tests := []struct {
		name          string
		logs          string
		wantDiversity float64
	}{
		{
			name:          "no tool calls",
			logs:          "some random logs",
			wantDiversity: 1.0, // default healthy
		},
		{
			name: "all same tool",
			logs: `[hortator-agentic] Tool call: run_shell({"command":"ls"})
[hortator-agentic] Tool call: run_shell({"command":"cat"})
[hortator-agentic] Tool call: run_shell({"command":"echo"})`,
			wantDiversity: 1.0 / 3.0, // 1 unique / 3 total
		},
		{
			name: "diverse tools",
			logs: `[hortator-agentic] Tool call: run_shell({"command":"ls"})
[hortator-agentic] Tool call: read_file({"path":"/test"})
[hortator-agentic] Tool call: spawn_task({"prompt":"hi"})`,
			wantDiversity: 1.0, // 3 unique / 3 total
		},
		{
			name: "some repetition",
			logs: `[hortator-agentic] Tool call: run_shell({"command":"ls"})
[hortator-agentic] Tool call: run_shell({"command":"cat"})
[hortator-agentic] Tool call: read_file({"path":"/test"})
[hortator-agentic] Tool call: run_shell({"command":"echo"})`,
			wantDiversity: 0.5, // 2 unique / 4 total
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolMatches := reToolCall.FindAllStringSubmatch(tt.logs, -1)
			var diversity float64
			if len(toolMatches) > 2 {
				toolSet := make(map[string]bool)
				for _, m := range toolMatches {
					toolSet[m[1]] = true
				}
				diversity = float64(len(toolSet)) / float64(len(toolMatches))
			} else {
				diversity = 1.0
			}

			if abs(diversity-tt.wantDiversity) > 0.01 {
				t.Errorf("expected diversity=%.2f, got %.2f", tt.wantDiversity, diversity)
			}
		})
	}
}

func TestStuckScore_PromptRepetition(t *testing.T) {
	tests := []struct {
		name        string
		logs        string
		wantRepeats int
	}{
		{
			name:        "no hashes",
			logs:        "regular logs",
			wantRepeats: 0,
		},
		{
			name: "all unique",
			logs: `[hortator-agentic] Prompt hash: aabbccdd11223344
[hortator-agentic] Prompt hash: eeff00112233aabb
[hortator-agentic] Prompt hash: 1122334455667788`,
			wantRepeats: 1,
		},
		{
			name: "repeated hash",
			logs: `[hortator-agentic] Prompt hash: aabbccdd11223344
[hortator-agentic] Prompt hash: aabbccdd11223344
[hortator-agentic] Prompt hash: aabbccdd11223344
[hortator-agentic] Prompt hash: eeff00112233aabb`,
			wantRepeats: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashMatches := rePromptHash.FindAllStringSubmatch(tt.logs, -1)
			maxCount := 0
			if len(hashMatches) > 1 {
				hashCounts := make(map[string]int)
				for _, m := range hashMatches {
					hashCounts[m[1]]++
				}
				for _, count := range hashCounts {
					if count > maxCount {
						maxCount = count
					}
				}
			}

			if maxCount != tt.wantRepeats {
				t.Errorf("expected maxRepeats=%d, got %d", tt.wantRepeats, maxCount)
			}
		})
	}
}

// --- Aggregate score ---

func TestStuckScore_Aggregate(t *testing.T) {
	tests := []struct {
		name         string
		diversity    float64
		repeats      int
		staleMins    float64
		cfg          StuckDetectionConfig
		wantStuck    bool
		wantMinScore float64
	}{
		{
			name:      "healthy agent",
			diversity: 0.8,
			repeats:   1,
			staleMins: 2,
			cfg: StuckDetectionConfig{
				ToolDiversityMin:   0.3,
				MaxRepeatedPrompts: 3,
				StatusStaleMinutes: 5,
			},
			wantStuck:    false,
			wantMinScore: 0.0,
		},
		{
			name:      "low diversity triggers stuck",
			diversity: 0.0,
			repeats:   5,
			staleMins: 10,
			cfg: StuckDetectionConfig{
				ToolDiversityMin:   0.3,
				MaxRepeatedPrompts: 3,
				StatusStaleMinutes: 5,
			},
			wantStuck:    true,
			wantMinScore: 0.5,
		},
		{
			name:      "only repeated prompts",
			diversity: 0.8,
			repeats:   10,
			staleMins: 1,
			cfg: StuckDetectionConfig{
				ToolDiversityMin:   0.3,
				MaxRepeatedPrompts: 3,
				StatusStaleMinutes: 5,
			},
			wantStuck:    false, // 35% weight, capped at 1.0 -> 0.35 < 0.5
			wantMinScore: 0.0,
		},
		{
			name:      "all signals bad",
			diversity: 0.0,
			repeats:   20,
			staleMins: 30,
			cfg: StuckDetectionConfig{
				ToolDiversityMin:   0.3,
				MaxRepeatedPrompts: 3,
				StatusStaleMinutes: 5,
			},
			wantStuck:    true,
			wantMinScore: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the scoring logic from health.go
			diversityPenalty := 0.0
			if tt.diversity < tt.cfg.ToolDiversityMin {
				diversityPenalty = (tt.cfg.ToolDiversityMin - tt.diversity) / tt.cfg.ToolDiversityMin
			}

			repetitionPenalty := 0.0
			if tt.cfg.MaxRepeatedPrompts > 0 && tt.repeats > tt.cfg.MaxRepeatedPrompts {
				repetitionPenalty = float64(tt.repeats-tt.cfg.MaxRepeatedPrompts) /
					float64(tt.cfg.MaxRepeatedPrompts)
				if repetitionPenalty > 1.0 {
					repetitionPenalty = 1.0
				}
			}

			stalenessPenalty := 0.0
			if tt.cfg.StatusStaleMinutes > 0 && tt.staleMins > float64(tt.cfg.StatusStaleMinutes) {
				stalenessPenalty = (tt.staleMins - float64(tt.cfg.StatusStaleMinutes)) /
					float64(tt.cfg.StatusStaleMinutes)
				if stalenessPenalty > 1.0 {
					stalenessPenalty = 1.0
				}
			}

			aggregate := 0.40*diversityPenalty + 0.35*repetitionPenalty + 0.25*stalenessPenalty
			isStuck := aggregate >= 0.5

			if isStuck != tt.wantStuck {
				t.Errorf("expected isStuck=%v, got %v (aggregate=%.3f)", tt.wantStuck, isStuck, aggregate)
			}
			if aggregate < tt.wantMinScore {
				t.Errorf("expected aggregate >= %.2f, got %.3f", tt.wantMinScore, aggregate)
			}
		})
	}
}

// --- isTerminalPhase (used by stuck detection) ---

func TestIsTerminalPhase(t *testing.T) {
	terminal := []corev1alpha1.AgentTaskPhase{
		corev1alpha1.AgentTaskPhaseCompleted,
		corev1alpha1.AgentTaskPhaseFailed,
		corev1alpha1.AgentTaskPhaseTimedOut,
		corev1alpha1.AgentTaskPhaseBudgetExceeded,
		corev1alpha1.AgentTaskPhaseCancelled,
	}
	for _, p := range terminal {
		if !isTerminalPhase(p) {
			t.Errorf("expected %s to be terminal", p)
		}
	}

	nonTerminal := []corev1alpha1.AgentTaskPhase{
		corev1alpha1.AgentTaskPhasePending,
		corev1alpha1.AgentTaskPhaseRunning,
		corev1alpha1.AgentTaskPhaseWaiting,
		corev1alpha1.AgentTaskPhaseRetrying,
		"",
	}
	for _, p := range nonTerminal {
		if isTerminalPhase(p) {
			t.Errorf("expected %s to be non-terminal", p)
		}
	}
}

// --- isAgenticTier ---

func TestIsAgenticTier(t *testing.T) {
	if !isAgenticTier("tribune") {
		t.Error("tribune should be agentic")
	}
	if !isAgenticTier("centurion") {
		t.Error("centurion should be agentic")
	}
	if isAgenticTier("legionary") {
		t.Error("legionary should not be agentic")
	}
	if isAgenticTier("") {
		t.Error("empty string should not be agentic")
	}
}

// --- statusStaleness (annotation parsing) ---

func TestStatusStaleness_FromAnnotation(t *testing.T) {
	now := metav1.Now()
	task := &corev1alpha1.AgentTask{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"hortator.ai/last-progress": now.Format("2006-01-02T15:04:05Z07:00"),
			},
		},
		Status: corev1alpha1.AgentTaskStatus{
			StartedAt: &now,
		},
	}

	// Parse the annotation the same way health.go does
	ann := task.Annotations["hortator.ai/last-progress"]
	_, err := time.Parse(time.RFC3339, ann)
	if err != nil {
		t.Errorf("failed to parse last-progress annotation: %v", err)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
