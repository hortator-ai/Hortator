/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package gateway

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9-]`)

// sanitizeName makes a string safe for use in a K8s resource name.
func sanitizeName(s string) string {
	s = strings.ToLower(s)
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.Trim(s, "-")
}

// buildPrompt concatenates chat messages into a single prompt string.
// System messages become context prefixes, user messages become the main prompt.
func buildPrompt(messages []Message) string {
	var systemParts []string
	var userParts []string

	for _, m := range messages {
		text := m.Content.String()
		switch m.Role {
		case "system":
			systemParts = append(systemParts, text)
		case "user":
			userParts = append(userParts, text)
		case "assistant":
			// Include assistant messages as conversation context
			userParts = append(userParts, fmt.Sprintf("[Previous assistant response:]\n%s", text))
		}
	}

	var parts []string
	if len(systemParts) > 0 {
		parts = append(parts, "[System Context]\n"+strings.Join(systemParts, "\n"))
	}
	parts = append(parts, userParts...)

	return strings.Join(parts, "\n\n")
}

// extractFiles collects all file parts from the request messages.
func extractFiles(messages []Message) []FileContent {
	var files []FileContent
	for _, m := range messages {
		files = append(files, m.Content.Files()...)
	}
	return files
}

// buildAgentTask creates an unstructured AgentTask from the request.
//
// Thread continuity (Level 1, future):
//
//	When sessions are implemented, this function should:
//	1. Accept a sessionID parameter
//	2. Set spec.storage.retain = true
//	3. Add label "hortator.ai/session" = sessionID
//	4. Set spec.storage.existingClaim = "session-<sessionID>" (if PVC exists)

// ModelConfig holds the LLM endpoint configuration resolved from an AgentRole.
type ModelConfig struct {
	Name       string
	Endpoint   string
	SecretName string
	SecretKey  string
}

func buildAgentTask(name, namespace, role, tier, prompt string, req *ChatCompletionRequest, modelCfg *ModelConfig, files []FileContent) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"prompt": prompt,
		"role":   role,
		"tier":   tier,
	}

	// Set model configuration from AgentRole
	if modelCfg != nil {
		model := map[string]interface{}{
			"name": modelCfg.Name,
		}
		if modelCfg.Endpoint != "" {
			model["endpoint"] = modelCfg.Endpoint
		}
		if modelCfg.SecretName != "" {
			model["apiKeyRef"] = map[string]interface{}{
				"secretName": modelCfg.SecretName,
				"key":        modelCfg.SecretKey,
			}
		}
		spec["model"] = model
	}

	if len(req.Capabilities) > 0 {
		caps := make([]interface{}, len(req.Capabilities))
		for i, c := range req.Capabilities {
			caps[i] = c
		}
		spec["capabilities"] = caps
	}

	if req.Budget != nil {
		budget := map[string]interface{}{}
		if req.Budget.MaxCostUsd != "" {
			budget["maxCostUsd"] = req.Budget.MaxCostUsd
		}
		if req.Budget.MaxTokens != nil {
			budget["maxTokens"] = *req.Budget.MaxTokens
		}
		spec["budget"] = budget
	}

	// Include input files if any were provided via content part arrays
	if len(files) > 0 {
		inputFiles := make([]interface{}, 0, len(files))
		for _, f := range files {
			inputFiles = append(inputFiles, map[string]interface{}{
				"filename": f.Filename,
				"data":     f.FileData,
			})
		}
		spec["inputFiles"] = inputFiles
	}

	task := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "core.hortator.ai/v1alpha1",
			"kind":       "AgentTask",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"hortator.ai/source": "gateway",
					"hortator.ai/role":   role,
					// Level 1 prep: session label will go here
					// "hortator.ai/session": sessionID,
				},
				"annotations": func() map[string]interface{} {
					ann := map[string]interface{}{}
					if req.NoCache {
						ann["hortator.ai/no-cache"] = "true"
					}
					return ann
				}(),
			},
			"spec": spec,
		},
	}

	return task
}

// extractTaskState reads status fields from an unstructured AgentTask.
func extractTaskState(obj *unstructured.Unstructured) *taskState {
	status, _, _ := unstructured.NestedMap(obj.Object, "status")
	if status == nil {
		return &taskState{Name: obj.GetName(), Phase: "Pending"}
	}

	state := &taskState{
		Name: obj.GetName(),
	}

	if v, ok := status["phase"].(string); ok {
		state.Phase = v
	}
	if v, ok := status["output"].(string); ok {
		state.Output = v
	}
	if v, ok := status["message"].(string); ok {
		state.Message = v
	}

	// Extract token usage
	if tokens, ok := status["tokensUsed"].(map[string]interface{}); ok {
		if v, ok := tokens["input"].(int64); ok {
			state.TokensIn = v
		} else if v, ok := tokens["input"].(float64); ok {
			state.TokensIn = int64(v)
		}
		if v, ok := tokens["output"].(int64); ok {
			state.TokensOut = v
		} else if v, ok := tokens["output"].(float64); ok {
			state.TokensOut = int64(v)
		}
	}

	// Extract child tasks
	if children, ok := status["childTasks"].([]interface{}); ok {
		for _, c := range children {
			if s, ok := c.(string); ok {
				state.Children = append(state.Children, s)
			}
		}
	}

	if v, ok := status["startedAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			state.StartedAt = &t
		}
	}

	return state
}

// isTerminalPhase returns true if the phase indicates the task is done.
func isTerminalPhase(phase string) bool {
	switch phase {
	case "Completed", "Failed", "TimedOut", "BudgetExceeded", "Cancelled":
		return true
	}
	return false
}

// mapPhaseToFinishReason translates an AgentTask phase to an OpenAI finish_reason.
func mapPhaseToFinishReason(phase string) string {
	switch phase {
	case "Completed":
		return "stop"
	case "BudgetExceeded":
		return "length" // Closest OpenAI equivalent for "ran out of budget"
	case "TimedOut":
		return "length"
	case "Failed", "Cancelled":
		return "stop" // OpenAI doesn't have "error" as a finish_reason
	default:
		return "stop"
	}
}
