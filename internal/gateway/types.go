/*
Copyright (c) 2026 GeneClackman
SPDX-License-Identifier: MIT
*/

// Package gateway implements the OpenAI-compatible API translation layer.
//
// Thread Continuity:
//
//	This package currently implements Level 0 (stateless). See cmd/gateway/main.go
//	for the full roadmap. When implementing Level 1, the key touchpoints are:
//	- ChatCompletions: read X-Hortator-Session header, map to PVC name
//	- createAgentTask: set spec.storage.retain=true, add session label
//	- A session cleanup controller (or cron) to GC expired session PVCs
package gateway

import "time"

// --- OpenAI API request/response types ---

// ChatCompletionRequest matches the OpenAI chat completion request schema.
// See: https://platform.openai.com/docs/api-reference/chat/create
type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`

	// Hortator extensions (ignored by OpenAI-compatible clients)
	Capabilities []string `json:"x_capabilities,omitempty"`
	Tier         string   `json:"x_tier,omitempty"`
	Budget       *Budget  `json:"x_budget,omitempty"`
	NoCache      bool     `json:"x_no_cache,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Budget struct {
	MaxCostUsd string `json:"max_cost_usd,omitempty"`
	MaxTokens  *int64 `json:"max_tokens,omitempty"`
}

// ChatCompletionResponse matches the OpenAI chat completion response schema.
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

type Choice struct {
	Index        int      `json:"index"`
	Message      *Message `json:"message,omitempty"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason *string  `json:"finish_reason,omitempty"`
}

type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// StreamChunk is a single SSE event in a streaming response.
type StreamChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   *Usage         `json:"usage,omitempty"`
}

type StreamChoice struct {
	Index        int     `json:"index"`
	Delta        Message `json:"delta"`
	FinishReason *string `json:"finish_reason,omitempty"`
}

// ModelObject matches the OpenAI model list response item.
type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelListResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

// ErrorResponse matches the OpenAI error response schema.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// AsyncTaskResponse is returned when X-Hortator-Async: true is set.
type AsyncTaskResponse struct {
	TaskID    string `json:"task_id"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

// ArtifactListResponse is returned by GET /api/v1/tasks/{id}/artifacts.
type ArtifactListResponse struct {
	TaskID    string `json:"task_id"`
	Phase     string `json:"phase"`
	Output    string `json:"output"`
	PVCStatus string `json:"pvc_status"`
	Note      string `json:"note,omitempty"`
}

// --- Internal types ---

// taskState tracks the lifecycle of an AgentTask being watched.
type taskState struct {
	Name      string
	Phase     string
	Output    string
	Message   string
	StartedAt *time.Time
	Children  []string
	TokensIn  int64
	TokensOut int64
}
