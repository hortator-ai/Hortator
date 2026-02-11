/*
Copyright (c) 2026 GeneClackman
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

	v1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
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

	// Level 1 prep: capture session header for future use
	// sessionID := r.Header.Get("X-Hortator-Session")
	// _ = sessionID // TODO(level-1): map to PVC name, set storage.retain

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

	// Create AgentTask
	taskName := fmt.Sprintf("gw-%s-%d", sanitizeName(role), time.Now().UnixMilli())
	task := buildAgentTask(taskName, h.Namespace, role, tier, prompt, &req, modelCfg)

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
			Message:      &Message{Role: "assistant", Content: state.Output},
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
			Delta: Message{Role: "assistant", Content: content},
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
