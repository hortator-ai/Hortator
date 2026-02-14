package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Compile-time interface checks.
var (
	_ Store = (*Qdrant)(nil)
	_ Store = (*Milvus)(nil)
)

func TestFactory(t *testing.T) {
	tests := []struct {
		provider string
		wantErr  bool
	}{
		{"qdrant", false},
		{"milvus", false},
		{"unknown", true},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			_, err := New(tt.provider, "http://localhost:6333")
			if (err != nil) != tt.wantErr {
				t.Errorf("New(%q) error = %v, wantErr %v", tt.provider, err, tt.wantErr)
			}
		})
	}
}

func TestFactoryEmptyEndpoint(t *testing.T) {
	_, err := New("qdrant", "")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
}

func TestQdrantHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	q, _ := NewQdrant(srv.URL)
	if err := q.Health(context.Background()); err != nil {
		t.Errorf("Health() = %v, want nil", err)
	}
}

func TestQdrantHealthUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	q, _ := NewQdrant(srv.URL)
	if err := q.Health(context.Background()); err == nil {
		t.Error("expected error for unhealthy server")
	}
}

func TestQdrantUpsertAndDelete(t *testing.T) {
	var upsertCalled, deleteCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/hortator-knowledge":
			// Collection exists.
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/collections/hortator-knowledge/points":
			upsertCalled = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode upsert body: %v", err)
			}
			points := body["points"].([]any)
			if len(points) != 1 {
				t.Errorf("expected 1 point, got %d", len(points))
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/hortator-knowledge/points/delete":
			deleteCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	q, _ := NewQdrant(srv.URL)
	ctx := context.Background()

	doc := Document{
		ID:        "test-doc",
		TaskID:    "task-1",
		Namespace: "default",
		Content:   "hello world",
		Embedding: make([]float32, 1536),
		Metadata:  map[string]string{"role": "engineer"},
	}

	if err := q.Upsert(ctx, doc); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}
	if !upsertCalled {
		t.Error("upsert endpoint not called")
	}

	if err := q.Delete(ctx, "test-doc"); err != nil {
		t.Fatalf("Delete() = %v", err)
	}
	if !deleteCalled {
		t.Error("delete endpoint not called")
	}
}

func TestQdrantSearchByVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/hortator-knowledge":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/hortator-knowledge/points/search":
			resp := map[string]any{
				"result": []map[string]any{
					{
						"id":    12345,
						"score": 0.95,
						"payload": map[string]string{
							"_id":        "doc-1",
							"_task_id":   "task-1",
							"_namespace": "default",
							"_content":   "test content",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	q, _ := NewQdrant(srv.URL)
	results, err := q.SearchByVector(context.Background(), make([]float32, 1536), 5, nil)
	if err != nil {
		t.Fatalf("SearchByVector() = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Document.ID != "doc-1" {
		t.Errorf("expected doc-1, got %s", results[0].Document.ID)
	}
	if results[0].Score != 0.95 {
		t.Errorf("expected score 0.95, got %f", results[0].Score)
	}
}

func TestQdrantAutoCreateCollection(t *testing.T) {
	var createCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/test-col":
			w.WriteHeader(http.StatusNotFound) // Collection doesn't exist.
		case r.Method == http.MethodPut && r.URL.Path == "/collections/test-col":
			createCalled = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			vectors := body["vectors"].(map[string]any)
			if int(vectors["size"].(float64)) != 768 {
				t.Errorf("expected dimension 768, got %v", vectors["size"])
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		default:
			fmt.Printf("unexpected: %s %s\n", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	q, _ := NewQdrant(srv.URL, WithCollection("test-col"), WithEmbeddingDimension(768))
	// Trigger ensureCollection via Health won't work (Health doesn't call ensure).
	// Use a direct call that triggers ensure.
	_ = q.Delete(context.Background(), "nonexistent")
	if !createCalled {
		t.Error("collection creation not triggered")
	}
}

func TestMilvusStub(t *testing.T) {
	m, err := NewMilvus("http://localhost:19530")
	if err != nil {
		t.Fatalf("NewMilvus() = %v", err)
	}

	ctx := context.Background()
	if err := m.Upsert(ctx, Document{}); err == nil {
		t.Error("expected not implemented error")
	}
	if _, err := m.Search(ctx, "", 5, nil); err == nil {
		t.Error("expected not implemented error")
	}
	if err := m.Delete(ctx, "x"); err == nil {
		t.Error("expected not implemented error")
	}
	if err := m.Health(ctx); err == nil {
		t.Error("expected not implemented error")
	}
}

func TestWithOptions(t *testing.T) {
	q, _ := NewQdrant("http://localhost:6333", WithCollection("custom"), WithEmbeddingDimension(768))
	if q.collection != "custom" {
		t.Errorf("expected collection 'custom', got %s", q.collection)
	}
	if q.dimension != 768 {
		t.Errorf("expected dimension 768, got %d", q.dimension)
	}
}
