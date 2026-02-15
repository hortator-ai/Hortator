/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var agentTaskGVR = schema.GroupVersionResource{
	Group:    "core.hortator.ai",
	Version:  "v1alpha1",
	Resource: "agenttasks",
}

var agentRoleGVR = schema.GroupVersionResource{
	Group:    "core.hortator.ai",
	Version:  "v1alpha1",
	Resource: "agentroles",
}

// Handler serves the OpenAI-compatible API endpoints.
type Handler struct {
	Namespace  string
	Clientset  kubernetes.Interface
	DynClient  dynamic.Interface
	AuthSecret string

	// Cached auth keys with TTL to avoid K8s API call on every HTTP request.
	authKeys map[string]bool
	authMu   sync.RWMutex
	authAt   time.Time
	authTTL  time.Duration // 0 means 60s default

	// RateLimiter enforces per-client request rate limits.
	RateLimiter *RateLimiter
}

// authenticate validates the Bearer token against a cached copy of the K8s Secret.
// The cache refreshes every 60s (configurable via authTTL) to avoid hitting the
// K8s API on every HTTP request while still picking up key rotations promptly.
func (h *Handler) authenticate(r *http.Request) error {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return fmt.Errorf("missing Authorization header")
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		return fmt.Errorf("invalid Authorization format, expected Bearer token")
	}
	token := strings.TrimPrefix(auth, "Bearer ")

	keys, err := h.getAuthKeys(r.Context())
	if err != nil {
		return err
	}
	if keys[token] {
		return nil
	}
	return fmt.Errorf("invalid API key")
}

// getAuthKeys returns cached auth keys, refreshing from K8s Secret if stale.
func (h *Handler) getAuthKeys(ctx context.Context) (map[string]bool, error) {
	ttl := h.authTTL
	if ttl == 0 {
		ttl = 60 * time.Second
	}

	h.authMu.RLock()
	if h.authKeys != nil && time.Since(h.authAt) < ttl {
		keys := h.authKeys
		h.authMu.RUnlock()
		return keys, nil
	}
	h.authMu.RUnlock()

	secret, err := h.Clientset.CoreV1().Secrets(h.Namespace).Get(
		ctx, h.AuthSecret, metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to read auth secret: %w", err)
	}

	keys := make(map[string]bool, len(secret.Data))
	for _, v := range secret.Data {
		keys[string(v)] = true
	}

	h.authMu.Lock()
	h.authKeys = keys
	h.authAt = time.Now()
	h.authMu.Unlock()

	return keys, nil
}

// writeError writes an OpenAI-compatible error response.
func writeError(w http.ResponseWriter, status int, msg, errType, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{Message: msg, Type: errType, Code: code},
	})
}

// ChatCompletions handles POST /v1/chat/completions.
//
// Thread continuity (Level 1, future):
//
//	When implementing sessions, read X-Hortator-Session here.
//	If present and valid, look up existing PVC by session label.
//	Set task.spec.storage = {retain: true, sessionId: <id>}.
//	The operator will mount the existing PVC instead of creating a new one.
func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	log := ctrl.Log.WithName("gateway.chat")

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	if err := h.authenticate(r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error(), "authentication_error", "invalid_api_key")
		return
	}

	// Per-client rate limiting
	if h.RateLimiter != nil && !h.RateLimiter.Allow(ClientKey(r)) {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded", "rate_limit_error", "rate_limit_exceeded")
		return
	}

	// Level 1 prep: capture session header for future use
	// Session continuity: map X-Hortator-Session header to PVC name and set
	// storage.retain so multi-turn conversations reuse the same workspace.
	// Tracked in roadmap/level-1-session-continuity.
	// sessionID := r.Header.Get("X-Hortator-Session")

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request_error", "invalid_body")
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required", "invalid_request_error", "missing_model")
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages is required", "invalid_request_error", "missing_messages")
		return
	}

	// Extract role from model field: "hortator/tech-lead" → "tech-lead"
	role := strings.TrimPrefix(req.Model, "hortator/")

	// Build prompt from messages (concatenate user messages, include system as context)
	prompt := buildPrompt(req.Messages)

	// Determine tier — default to tribune (the entry point for decomposition).
	// The tribune decides whether to delegate or answer directly.
	tier := "tribune"
	if req.Tier != "" {
		tier = req.Tier
	}

	// Resolve model config from AgentRole (if it exists)
	modelCfg := h.resolveModelConfig(r.Context(), role)

	// Extract files from content part arrays
	files := extractFiles(req.Messages)

	// Create AgentTask
	taskName := fmt.Sprintf("gw-%s-%d", sanitizeName(role), time.Now().UnixMilli())
	task := buildAgentTask(taskName, h.Namespace, role, tier, prompt, &req, modelCfg, files)

	log.Info("audit: chat.completions", "role", role, "tier", tier, "stream", req.Stream, "task", taskName)

	created, err := h.DynClient.Resource(agentTaskGVR).Namespace(h.Namespace).Create(
		r.Context(), task, metav1.CreateOptions{},
	)
	if err != nil {
		log.Error(err, "failed to create AgentTask")
		writeError(w, http.StatusInternalServerError, "failed to create task: "+err.Error(), "server_error", "task_creation_failed")
		return
	}

	taskName = created.GetName()

	// Async mode: return task ID immediately without waiting for completion.
	// Caller polls GET /api/v1/tasks/{id}/artifacts for results.
	if r.Header.Get("X-Hortator-Async") == "true" {
		log.Info("audit: chat.completions.async", "task", taskName)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AsyncTaskResponse{
			TaskID:    taskName,
			Namespace: h.Namespace,
			Status:    "Pending",
			Message:   "Task created. Poll /api/v1/tasks/" + taskName + "/artifacts for results.",
		})
		return
	}

	if req.Stream {
		h.streamResponse(r.Context(), w, taskName, req.Model)
	} else {
		h.blockingResponse(r.Context(), w, taskName, req.Model)
	}
}

// blockingResponse waits for task completion and returns a single JSON response.
func (h *Handler) blockingResponse(ctx context.Context, w http.ResponseWriter, taskName, model string) {
	log := ctrl.Log.WithName("gateway.blocking")

	state, err := h.watchTaskUntilDone(ctx, taskName)
	if err != nil {
		log.Error(err, "watch failed", "task", taskName)
		writeError(w, http.StatusGatewayTimeout, "task watch failed: "+err.Error(), "server_error", "watch_failed")
		return
	}

	finishReason := mapPhaseToFinishReason(state.Phase)

	resp := ChatCompletionResponse{
		ID:      "chatcmpl-" + taskName,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []Choice{{
			Index:        0,
			Message:      &Message{Role: "assistant", Content: MessageContent{Text: state.Output}},
			FinishReason: &finishReason,
		}},
		Usage: &Usage{
			PromptTokens:     state.TokensIn,
			CompletionTokens: state.TokensOut,
			TotalTokens:      state.TokensIn + state.TokensOut,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// streamResponse sends SSE chunks as the task progresses.
func (h *Handler) streamResponse(ctx context.Context, w http.ResponseWriter, taskName, model string) {
	log := ctrl.Log.WithName("gateway.stream")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported", "server_error", "no_flusher")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	chunkID := "chatcmpl-" + taskName
	created := time.Now().Unix()

	// Send initial acknowledgment
	h.sendStreamChunk(w, flusher, chunkID, model, created, fmt.Sprintf("[task %s created, waiting for agent...]\n", taskName))

	watcher, err := h.DynClient.Resource(agentTaskGVR).Namespace(h.Namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + taskName,
	})
	if err != nil {
		log.Error(err, "failed to start watch")
		h.sendStreamChunk(w, flusher, chunkID, model, created, "[error: failed to watch task]\n")
		h.sendStreamDone(w, flusher)
		return
	}
	defer watcher.Stop()

	lastPhase := ""
	lastMessage := ""

	for {
		select {
		case <-ctx.Done():
			h.sendStreamDone(w, flusher)
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				h.sendStreamDone(w, flusher)
				return
			}
			if event.Type == watch.Error {
				h.sendStreamChunk(w, flusher, chunkID, model, created, "[error: watch error]\n")
				h.sendStreamDone(w, flusher)
				return
			}

			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}

			state := extractTaskState(obj)

			// Send progress updates on phase or message changes
			if state.Phase != lastPhase {
				h.sendStreamChunk(w, flusher, chunkID, model, created,
					fmt.Sprintf("[%s: %s]\n", state.Phase, state.Message))
				lastPhase = state.Phase
				lastMessage = state.Message
			} else if state.Message != lastMessage && state.Message != "" {
				h.sendStreamChunk(w, flusher, chunkID, model, created,
					fmt.Sprintf("[%s]\n", state.Message))
				lastMessage = state.Message
			}

			// Report child tasks
			for _, child := range state.Children {
				h.sendStreamChunk(w, flusher, chunkID, model, created,
					fmt.Sprintf("[child: %s]\n", child))
			}

			// Terminal state — send final output
			if isTerminalPhase(state.Phase) {
				if state.Output != "" {
					h.sendStreamChunk(w, flusher, chunkID, model, created, state.Output)
				}

				// Send final chunk with finish_reason and usage
				finishReason := mapPhaseToFinishReason(state.Phase)
				chunk := StreamChunk{
					ID:      chunkID,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   model,
					Choices: []StreamChoice{{
						Index:        0,
						Delta:        Message{},
						FinishReason: &finishReason,
					}},
					Usage: &Usage{
						PromptTokens:     state.TokensIn,
						CompletionTokens: state.TokensOut,
						TotalTokens:      state.TokensIn + state.TokensOut,
					},
				}
				data, _ := json.Marshal(chunk)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()

				h.sendStreamDone(w, flusher)
				return
			}
		}
	}
}

// watchTaskUntilDone blocks until the AgentTask reaches a terminal phase.
func (h *Handler) watchTaskUntilDone(ctx context.Context, taskName string) (*taskState, error) {
	watcher, err := h.DynClient.Resource(agentTaskGVR).Namespace(h.Namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + taskName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start watch: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return nil, fmt.Errorf("watch channel closed")
			}
			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}
			state := extractTaskState(obj)
			if isTerminalPhase(state.Phase) {
				return state, nil
			}
		}
	}
}

// sendStreamChunk writes a single SSE data event with content.
func (h *Handler) sendStreamChunk(w http.ResponseWriter, flusher http.Flusher, id, model string, created int64, content string) {
	chunk := StreamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []StreamChoice{{
			Index: 0,
			Delta: Message{Role: "assistant", Content: MessageContent{Text: content}},
		}},
	}
	data, _ := json.Marshal(chunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// sendStreamDone writes the final [DONE] SSE event.
func (h *Handler) sendStreamDone(w http.ResponseWriter, flusher http.Flusher) {
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// convertToAgentRole converts an unstructured object to a typed AgentRole.
func convertToAgentRole(obj *unstructured.Unstructured) (*v1alpha1.AgentRole, error) {
	role := &v1alpha1.AgentRole{}
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, role)
	return role, err
}

// resolveModelConfig looks up the AgentRole and returns model configuration.
// Falls back to a default config if the role doesn't exist.
func (h *Handler) resolveModelConfig(ctx context.Context, roleName string) *ModelConfig {
	log := ctrl.Log.WithName("gateway.resolve")

	role, err := h.DynClient.Resource(agentRoleGVR).Namespace(h.Namespace).Get(ctx, roleName, metav1.GetOptions{})
	if err != nil {
		log.V(1).Info("AgentRole not found, using defaults", "role", roleName, "error", err)
		// Fall back: check if anthropic-api-key secret exists in namespace
		_, secretErr := h.Clientset.CoreV1().Secrets(h.Namespace).Get(ctx, "anthropic-api-key", metav1.GetOptions{})
		if secretErr == nil {
			return &ModelConfig{
				Name:       "claude-sonnet-4-20250514",
				Endpoint:   "https://api.anthropic.com/v1",
				SecretName: "anthropic-api-key",
				SecretKey:  "api-key",
			}
		}
		return nil
	}

	typedRole, err := convertToAgentRole(role)
	if err != nil {
		log.Error(err, "failed to convert AgentRole to typed object", "role", roleName)
		return nil
	}

	cfg := &ModelConfig{}
	cfg.Name = typedRole.Spec.DefaultModel
	cfg.Endpoint = typedRole.Spec.DefaultEndpoint
	if typedRole.Spec.ApiKeyRef != nil {
		cfg.SecretName = typedRole.Spec.ApiKeyRef.SecretName
		cfg.SecretKey = typedRole.Spec.ApiKeyRef.Key
	}

	// If role has a model name but no explicit endpoint/secret, infer from model name
	if cfg.Name != "" && cfg.Endpoint == "" {
		if strings.Contains(cfg.Name, "claude") {
			cfg.Endpoint = "https://api.anthropic.com/v1"
			if cfg.SecretName == "" {
				cfg.SecretName = "anthropic-api-key"
				cfg.SecretKey = "api-key"
			}
		} else if strings.Contains(cfg.Name, "gpt") {
			cfg.Endpoint = "https://api.openai.com/v1"
			if cfg.SecretName == "" {
				cfg.SecretName = "openai-api-key"
				cfg.SecretKey = "api-key"
			}
		}
	}

	return cfg
}

// TaskArtifacts handles GET /api/v1/tasks/{id}/artifacts.
// Serves files from the completed task's PVC via /outbox/artifacts/.
// Returns 404 if task not found, 409 if not terminal, 410 if PVC gone.
func (h *Handler) TaskArtifacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	if err := h.authenticate(r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error(), "authentication_error", "invalid_api_key")
		return
	}

	log := ctrl.Log.WithName("gateway.artifacts")

	// Extract task ID from URL path: /api/v1/tasks/{id}/artifacts
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 5 {
		writeError(w, http.StatusBadRequest, "invalid path: expected /api/v1/tasks/{id}/artifacts", "invalid_request_error", "bad_path")
		return
	}
	taskName := pathParts[3]

	// Fetch the task
	task, err := h.DynClient.Resource(agentTaskGVR).Namespace(h.Namespace).Get(
		r.Context(), taskName, metav1.GetOptions{},
	)
	if err != nil {
		log.V(1).Info("Task not found", "task", taskName, "error", err)
		writeError(w, http.StatusNotFound, "task not found: "+taskName, "not_found", "task_not_found")
		return
	}

	state := extractTaskState(task)
	if !isTerminalPhase(state.Phase) {
		writeError(w, http.StatusConflict, "task is not complete: "+state.Phase, "invalid_request_error", "task_not_terminal")
		return
	}

	// List the artifacts by executing into the PVC.
	// For now, return artifact metadata from the task status output.
	// Full PVC file serving is a deeper integration that requires mounting the PVC.
	artifacts := ArtifactListResponse{
		TaskID: taskName,
		Phase:  state.Phase,
		Output: state.Output,
		Note:   "Full artifact file download requires PVC access. Use 'hortator result --artifacts' from within the cluster.",
	}

	// Try to list PVC contents via a pod exec (best effort)
	pvcName := taskName + "-storage"
	pvc, pvcErr := h.Clientset.CoreV1().PersistentVolumeClaims(h.Namespace).Get(
		r.Context(), pvcName, metav1.GetOptions{},
	)
	if pvcErr != nil || pvc.DeletionTimestamp != nil {
		artifacts.Note = "PVC has been cleaned up. Only status.output is available."
		artifacts.PVCStatus = "gone"
	} else {
		artifacts.PVCStatus = string(pvc.Status.Phase)
	}

	log.Info("audit: artifacts.list", "task", taskName, "pvc_status", artifacts.PVCStatus)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(artifacts)
}

// ListModels handles GET /v1/models.
// Returns AgentRoles in the namespace as available "models".
// If no AgentRoles exist, returns a default entry.
func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	if err := h.authenticate(r); err != nil {
		writeError(w, http.StatusUnauthorized, err.Error(), "authentication_error", "invalid_api_key")
		return
	}

	log := ctrl.Log.WithName("gateway.models")

	roles, err := h.DynClient.Resource(agentRoleGVR).Namespace(h.Namespace).List(
		r.Context(), metav1.ListOptions{},
	)

	var models []ModelObject
	if err == nil {
		for _, role := range roles.Items {
			models = append(models, ModelObject{
				ID:      "hortator/" + role.GetName(),
				Object:  "model",
				Created: role.GetCreationTimestamp().Unix(),
				OwnedBy: "hortator",
			})
		}
	}

	// Always include a default model
	if len(models) == 0 {
		models = append(models, ModelObject{
			ID:      "hortator/default",
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "hortator",
		})
	}

	log.Info("audit: list.models", "count", len(models))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ModelListResponse{
		Object: "list",
		Data:   models,
	})
}
