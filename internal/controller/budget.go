/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

const (
	litellmPriceMapURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
)

// ModelPricing holds per-token prices for a model.
type ModelPricing struct {
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
}

// PriceMap is a thread-safe cache of model pricing data from LiteLLM.
type PriceMap struct {
	mu        sync.RWMutex
	prices    map[string]ModelPricing
	fetchedAt time.Time
	refreshH  int // refresh interval in hours
}

// NewPriceMap creates a new empty price map with the given refresh interval.
func NewPriceMap(refreshIntervalHours int) *PriceMap {
	if refreshIntervalHours <= 0 {
		refreshIntervalHours = 24
	}
	return &PriceMap{
		prices:   make(map[string]ModelPricing),
		refreshH: refreshIntervalHours,
	}
}

// RefreshIfStale fetches the LiteLLM price map if the cache has expired.
func (pm *PriceMap) RefreshIfStale() {
	pm.mu.RLock()
	age := time.Since(pm.fetchedAt)
	pm.mu.RUnlock()

	if age < time.Duration(pm.refreshH)*time.Hour && len(pm.prices) > 0 {
		return
	}

	pm.fetch()
}

func (pm *PriceMap) fetch() {
	logger := log.Log.WithName("budget.pricemap")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(litellmPriceMapURL)
	if err != nil {
		logger.Error(err, "Failed to fetch LiteLLM price map, using cached data")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		logger.Info("Non-200 response from LiteLLM price map", "status", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		logger.Error(err, "Failed to read LiteLLM price map body")
		return
	}

	// The LiteLLM JSON maps model names to objects containing pricing fields.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		logger.Error(err, "Failed to parse LiteLLM price map JSON")
		return
	}

	prices := make(map[string]ModelPricing, len(raw))
	for modelName, data := range raw {
		var entry struct {
			InputCostPerToken  *float64 `json:"input_cost_per_token"`
			OutputCostPerToken *float64 `json:"output_cost_per_token"`
		}
		if err := json.Unmarshal(data, &entry); err != nil {
			continue // skip non-model entries
		}
		if entry.InputCostPerToken == nil && entry.OutputCostPerToken == nil {
			continue
		}
		p := ModelPricing{}
		if entry.InputCostPerToken != nil {
			p.InputCostPerToken = *entry.InputCostPerToken
		}
		if entry.OutputCostPerToken != nil {
			p.OutputCostPerToken = *entry.OutputCostPerToken
		}
		prices[modelName] = p
	}

	pm.mu.Lock()
	pm.prices = prices
	pm.fetchedAt = time.Now()
	pm.mu.Unlock()

	logger.Info("Refreshed LiteLLM price map", "models", len(prices))
}

// GetPricing returns the pricing for a model, trying exact match first
// and then common prefix variations (e.g. "anthropic/claude-sonnet-4-20250514").
func (pm *PriceMap) GetPricing(model string) (ModelPricing, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Exact match
	if p, ok := pm.prices[model]; ok {
		return p, true
	}

	// Common provider prefixes used by litellm
	prefixes := []string{
		"anthropic/", "openai/", "azure/", "google/",
		"bedrock/", "vertex_ai/", "groq/", "together_ai/",
	}
	for _, prefix := range prefixes {
		if p, ok := pm.prices[prefix+model]; ok {
			return p, true
		}
	}

	return ModelPricing{}, false
}

// CalculateCost computes the estimated cost in USD for the given token usage.
func (pm *PriceMap) CalculateCost(model string, tokensIn, tokensOut int64) (float64, error) {
	pricing, ok := pm.GetPricing(model)
	if !ok {
		return 0, fmt.Errorf("no pricing found for model %q", model)
	}

	cost := float64(tokensIn)*pricing.InputCostPerToken +
		float64(tokensOut)*pricing.OutputCostPerToken
	return cost, nil
}

// CalculateTaskCost computes estimated cost for an AgentTask using its token usage
// and model name. Returns the cost as a formatted string (e.g. "0.0342") or empty
// string if cost cannot be calculated.
func (pm *PriceMap) CalculateTaskCost(task *corev1alpha1.AgentTask) string {
	if task.Status.TokensUsed == nil {
		return ""
	}

	model := ""
	if task.Spec.Model != nil {
		model = task.Spec.Model.Name
	}
	if model == "" {
		return ""
	}

	cost, err := pm.CalculateCost(model, task.Status.TokensUsed.Input, task.Status.TokensUsed.Output)
	if err != nil {
		return ""
	}

	return strconv.FormatFloat(cost, 'f', 6, 64)
}

// IsBudgetExceeded checks if the task's accumulated cost exceeds its budget.
func IsBudgetExceeded(task *corev1alpha1.AgentTask, estimatedCost float64) bool {
	if task.Spec.Budget == nil {
		return false
	}

	// Check token budget
	if task.Spec.Budget.MaxTokens != nil && task.Status.TokensUsed != nil {
		totalTokens := task.Status.TokensUsed.Input + task.Status.TokensUsed.Output
		if totalTokens >= *task.Spec.Budget.MaxTokens {
			return true
		}
	}

	// Check cost budget
	if task.Spec.Budget.MaxCostUsd != "" && estimatedCost > 0 {
		maxCost, err := strconv.ParseFloat(task.Spec.Budget.MaxCostUsd, 64)
		if err == nil && estimatedCost >= maxCost {
			return true
		}
	}

	return false
}
