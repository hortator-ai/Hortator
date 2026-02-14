package vectorstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// Qdrant implements Store using Qdrant's REST API.
type Qdrant struct {
	endpoint   string
	collection string
	dimension  int
	client     *http.Client

	ensureOnce sync.Once
	ensureErr  error
}

// NewQdrant creates a Qdrant-backed vector store.
func NewQdrant(endpoint string, opts ...Option) (*Qdrant, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("qdrant endpoint is required")
	}
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	return &Qdrant{
		endpoint:   endpoint,
		collection: o.Collection,
		dimension:  o.EmbeddingDimension,
		client:     &http.Client{},
	}, nil
}

// ensureCollection creates the collection if it doesn't exist.
func (q *Qdrant) ensureCollection(ctx context.Context) error {
	q.ensureOnce.Do(func() {
		// Check if collection exists.
		url := fmt.Sprintf("%s/collections/%s", q.endpoint, q.collection)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			q.ensureErr = err
			return
		}
		resp, err := q.client.Do(req)
		if err != nil {
			q.ensureErr = err
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return
		}

		// Create collection.
		body := map[string]any{
			"vectors": map[string]any{
				"size":     q.dimension,
				"distance": "Cosine",
			},
		}
		data, _ := json.Marshal(body)
		req, err = http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
		if err != nil {
			q.ensureErr = err
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err = q.client.Do(req)
		if err != nil {
			q.ensureErr = err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(resp.Body)
			q.ensureErr = fmt.Errorf("failed to create collection: %s %s", resp.Status, string(b))
		}
	})
	return q.ensureErr
}

// docID produces a deterministic uint64 hash for use as a Qdrant point ID.
func docID(id string) uint64 {
	var h uint64 = 14695981039346656037 // FNV-1a offset basis
	for i := 0; i < len(id); i++ {
		h ^= uint64(id[i])
		h *= 1099511628211 // FNV-1a prime
	}
	return h
}

func (q *Qdrant) Upsert(ctx context.Context, doc Document) error {
	if err := q.ensureCollection(ctx); err != nil {
		return fmt.Errorf("ensure collection: %w", err)
	}

	payload := doc.Metadata
	if payload == nil {
		payload = make(map[string]string)
	}
	payload["_id"] = doc.ID
	payload["_task_id"] = doc.TaskID
	payload["_namespace"] = doc.Namespace
	payload["_content"] = doc.Content

	point := map[string]any{
		"id":      docID(doc.ID),
		"vector":  doc.Embedding,
		"payload": payload,
	}

	body := map[string]any{"points": []any{point}}
	data, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/collections/%s/points", q.endpoint, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant upsert failed: %s %s", resp.Status, string(b))
	}
	return nil
}

func (q *Qdrant) Search(ctx context.Context, query string, topK int, filter map[string]string) ([]SearchResult, error) {
	if err := q.ensureCollection(ctx); err != nil {
		return nil, fmt.Errorf("ensure collection: %w", err)
	}

	// Text-based search requires embedding generation, which is out of scope
	// for this layer. Callers should use SearchByVector with a pre-computed
	// embedding, or wrap this store with an embedding middleware.
	return nil, fmt.Errorf("text-based search not yet implemented; caller must provide embedding vector via SearchByVector")
}

// SearchByVector searches using a pre-computed embedding vector.
func (q *Qdrant) SearchByVector(ctx context.Context, vector []float32, topK int, filter map[string]string) ([]SearchResult, error) {
	if err := q.ensureCollection(ctx); err != nil {
		return nil, fmt.Errorf("ensure collection: %w", err)
	}

	body := map[string]any{
		"vector":       vector,
		"top":          topK,
		"with_payload": true,
	}

	if len(filter) > 0 {
		var must []any
		for k, v := range filter {
			must = append(must, map[string]any{
				"key":   k,
				"match": map[string]any{"value": v},
			})
		}
		body["filter"] = map[string]any{"must": must}
	}

	data, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/collections/%s/points/search", q.endpoint, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant search failed: %s %s", resp.Status, string(b))
	}

	var result struct {
		Result []struct {
			ID      uint64            `json:"id"`
			Score   float32           `json:"score"`
			Payload map[string]string `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	var results []SearchResult
	for _, r := range result.Result {
		doc := Document{
			ID:        r.Payload["_id"],
			TaskID:    r.Payload["_task_id"],
			Namespace: r.Payload["_namespace"],
			Content:   r.Payload["_content"],
			Metadata:  r.Payload,
		}
		results = append(results, SearchResult{Document: doc, Score: r.Score})
	}
	return results, nil
}

func (q *Qdrant) Delete(ctx context.Context, id string) error {
	if err := q.ensureCollection(ctx); err != nil {
		return fmt.Errorf("ensure collection: %w", err)
	}

	body := map[string]any{
		"points": []uint64{docID(id)},
	}
	data, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/collections/%s/points/delete", q.endpoint, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant delete failed: %s %s", resp.Status, string(b))
	}
	return nil
}

func (q *Qdrant) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/healthz", q.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("qdrant unhealthy: %s", resp.Status)
	}
	return nil
}
