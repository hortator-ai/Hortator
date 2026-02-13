/*
Copyright (c) 2026 hortator-ai
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

import (
	"encoding/json"
	"time"
)

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

// Message represents a chat message with OpenAI-compatible content.
// Content can be a plain string or an array of ContentPart objects.
type Message struct {
	Role    string         `json:"role"`
	Content MessageContent `json:"content"`
}

// MessageContent wraps string or []ContentPart for OpenAI-compatible content.
type MessageContent struct {
	Text  string        // plain string content
	Parts []ContentPart // array of typed parts
}

// ContentPart represents a typed content element (text or file).
type ContentPart struct {
	Type string       `json:"type"` // "text" or "file"
	Text string       `json:"text,omitempty"`
	File *FileContent `json:"file,omitempty"`
}

// FileContent holds base64-encoded file data.
type FileContent struct {
	FileData string `json:"file_data"` // base64-encoded content
	Filename string `json:"filename"`
}

// UnmarshalJSON implements custom unmarshalling for MessageContent.
// Accepts either a plain string or an array of ContentPart objects.
func (mc *MessageContent) UnmarshalJSON(data []byte) error {
	// Try plain string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		mc.Text = s
		return nil
	}

	// Try array of content parts
	var parts []ContentPart
	if err := json.Unmarshal(data, &parts); err == nil {
		mc.Parts = parts
		// Also extract concatenated text for backward compatibility
		for _, p := range parts {
			if p.Type == "text" {
				if mc.Text != "" {
					mc.Text += "\n"
				}
				mc.Text += p.Text
			}
		}
		return nil
	}

	return json.Unmarshal(data, &s) // return original error
}

// MarshalJSON implements custom marshalling for MessageContent.
func (mc MessageContent) MarshalJSON() ([]byte, error) {
	if len(mc.Parts) > 0 {
		return json.Marshal(mc.Parts)
	}
	return json.Marshal(mc.Text)
}

// String returns the text content of the message.
func (mc MessageContent) String() string {
	return mc.Text
}

// Files returns all file parts from the content.
func (mc MessageContent) Files() []FileContent {
	var files []FileContent
	for _, p := range mc.Parts {
		if p.Type == "file" && p.File != nil {
			files = append(files, *p.File)
		}
	}
	return files
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
