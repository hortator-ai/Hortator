/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func newDynScheme() *k8sruntime.Scheme {
	s := k8sruntime.NewScheme()
	s.AddKnownTypeWithName(agentRoleGVR.GroupVersion().WithKind("AgentRoleList"), &unstructured.UnstructuredList{})
	s.AddKnownTypeWithName(agentTaskGVR.GroupVersion().WithKind("AgentTaskList"), &unstructured.UnstructuredList{})
	return s
}

func newTestHandler(secrets ...*corev1.Secret) *Handler {
	objs := make([]k8sruntime.Object, len(secrets))
	for i, s := range secrets {
		objs[i] = s
	}
	return &Handler{
		Namespace:  "default",
		Clientset:  k8sfake.NewClientset(objs...),
		DynClient:  dynamicfake.NewSimpleDynamicClient(newDynScheme()),
		AuthSecret: "gateway-keys",
	}
}

func authSecret(keys ...string) *corev1.Secret {
	data := make(map[string][]byte, len(keys))
	for i, k := range keys {
		data["key-"+string(rune('0'+i))] = []byte(k)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "gateway-keys", Namespace: "default"},
		Data:       data,
	}
}

func TestChatCompletions_MethodNotAllowed(t *testing.T) {
	h := newTestHandler(authSecret("test-key"))
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	h.ChatCompletions(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestChatCompletions_MissingAuth(t *testing.T) {
	h := newTestHandler(authSecret("test-key"))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	h.ChatCompletions(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestChatCompletions_InvalidBody(t *testing.T) {
	h := newTestHandler(authSecret("test-key"))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	h.ChatCompletions(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestChatCompletions_EmptyModel(t *testing.T) {
	h := newTestHandler(authSecret("test-key"))
	body := `{"model":"","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	h.ChatCompletions(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestChatCompletions_EmptyMessages(t *testing.T) {
	h := newTestHandler(authSecret("test-key"))
	body := `{"model":"hortator/test","messages":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	h.ChatCompletions(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListModels_MethodNotAllowed(t *testing.T) {
	h := newTestHandler(authSecret("test-key"))
	req := httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	w := httptest.NewRecorder()
	h.ListModels(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestListModels_MissingAuth(t *testing.T) {
	h := newTestHandler(authSecret("test-key"))
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	h.ListModels(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestListModels_DefaultModel(t *testing.T) {
	h := newTestHandler(authSecret("test-key"))
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	h.ListModels(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp ModelListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("expected at least one model")
	}
	if resp.Data[0].ID != "hortator/default" {
		t.Errorf("model id = %q, want hortator/default", resp.Data[0].ID)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, 422, "test msg", "test_type", "test_code")
	if w.Code != 422 {
		t.Errorf("status = %d, want 422", w.Code)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if resp.Error.Message != "test msg" {
		t.Errorf("message = %q", resp.Error.Message)
	}
	if resp.Error.Type != "test_type" {
		t.Errorf("type = %q", resp.Error.Type)
	}
	if resp.Error.Code != "test_code" {
		t.Errorf("code = %q", resp.Error.Code)
	}
}

func TestResolveModelConfig(t *testing.T) {
	t.Run("role with full config", func(t *testing.T) {
		role := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "core.hortator.ai/v1alpha1",
				"kind":       "AgentRole",
				"metadata":   map[string]interface{}{"name": "dev", "namespace": "default"},
				"spec": map[string]interface{}{
					"defaultModel":    "claude-sonnet-4-20250514",
					"defaultEndpoint": "https://api.anthropic.com/v1",
					"apiKeyRef": map[string]interface{}{
						"secretName": "my-key",
						"key":        "api-key",
					},
				},
			},
		}
		scheme := k8sruntime.NewScheme()
		dynClient := dynamicfake.NewSimpleDynamicClient(scheme, role)
		h := &Handler{
			Namespace: "default",
			Clientset: k8sfake.NewClientset(),
			DynClient: dynClient,
		}
		cfg := h.resolveModelConfig(context.TODO(), "dev")
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
		if cfg.Name != "claude-sonnet-4-20250514" {
			t.Errorf("name = %q", cfg.Name)
		}
		if cfg.Endpoint != "https://api.anthropic.com/v1" {
			t.Errorf("endpoint = %q", cfg.Endpoint)
		}
		if cfg.SecretName != "my-key" {
			t.Errorf("secretName = %q", cfg.SecretName)
		}
	})

	t.Run("role with model name only infers anthropic", func(t *testing.T) {
		role := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "core.hortator.ai/v1alpha1",
				"kind":       "AgentRole",
				"metadata":   map[string]interface{}{"name": "dev", "namespace": "default"},
				"spec": map[string]interface{}{
					"defaultModel": "claude-sonnet",
				},
			},
		}
		scheme := k8sruntime.NewScheme()
		dynClient := dynamicfake.NewSimpleDynamicClient(scheme, role)
		h := &Handler{Namespace: "default", Clientset: k8sfake.NewClientset(), DynClient: dynClient}
		cfg := h.resolveModelConfig(context.TODO(), "dev")
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
		if cfg.Endpoint != "https://api.anthropic.com/v1" {
			t.Errorf("endpoint = %q, want anthropic", cfg.Endpoint)
		}
		if cfg.SecretName != "anthropic-api-key" {
			t.Errorf("secretName = %q", cfg.SecretName)
		}
	})

	t.Run("role with gpt model infers openai", func(t *testing.T) {
		role := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "core.hortator.ai/v1alpha1",
				"kind":       "AgentRole",
				"metadata":   map[string]interface{}{"name": "dev", "namespace": "default"},
				"spec":       map[string]interface{}{"defaultModel": "gpt-4o"},
			},
		}
		scheme := k8sruntime.NewScheme()
		dynClient := dynamicfake.NewSimpleDynamicClient(scheme, role)
		h := &Handler{Namespace: "default", Clientset: k8sfake.NewClientset(), DynClient: dynClient}
		cfg := h.resolveModelConfig(context.TODO(), "dev")
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
		if cfg.Endpoint != "https://api.openai.com/v1" {
			t.Errorf("endpoint = %q", cfg.Endpoint)
		}
	})

	t.Run("role not found with anthropic secret fallback", func(t *testing.T) {
		scheme := k8sruntime.NewScheme()
		dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "anthropic-api-key", Namespace: "default"},
			Data:       map[string][]byte{"api-key": []byte("sk-test")},
		}
		h := &Handler{
			Namespace: "default",
			Clientset: k8sfake.NewClientset(secret),
			DynClient: dynClient,
		}
		cfg := h.resolveModelConfig(context.TODO(), "nonexistent")
		if cfg == nil {
			t.Fatal("expected anthropic fallback config")
		}
		if cfg.SecretName != "anthropic-api-key" {
			t.Errorf("secretName = %q", cfg.SecretName)
		}
	})

	t.Run("role not found no secret returns nil", func(t *testing.T) {
		scheme := k8sruntime.NewScheme()
		dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
		h := &Handler{
			Namespace: "default",
			Clientset: k8sfake.NewClientset(),
			DynClient: dynClient,
		}
		cfg := h.resolveModelConfig(context.TODO(), "nonexistent")
		if cfg != nil {
			t.Errorf("expected nil, got %+v", cfg)
		}
	})
}
