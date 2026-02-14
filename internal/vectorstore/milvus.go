package vectorstore

import (
	"context"
	"fmt"
)

// TODO: Implement full Milvus vector store client.
// This is a stub that satisfies the Store interface.

// Milvus is a stub Store implementation for Milvus.
type Milvus struct {
	endpoint   string
	collection string
	dimension  int
}

// NewMilvus creates a Milvus-backed vector store (stub).
func NewMilvus(endpoint string, opts ...Option) (*Milvus, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("milvus endpoint is required")
	}
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	return &Milvus{
		endpoint:   endpoint,
		collection: o.Collection,
		dimension:  o.EmbeddingDimension,
	}, nil
}

func (m *Milvus) Upsert(ctx context.Context, doc Document) error {
	// TODO: Implement Milvus upsert via REST or gRPC API.
	return fmt.Errorf("milvus: not implemented")
}

func (m *Milvus) Search(ctx context.Context, query string, topK int, filter map[string]string) ([]SearchResult, error) {
	// TODO: Implement Milvus search.
	return nil, fmt.Errorf("milvus: not implemented")
}

func (m *Milvus) Delete(ctx context.Context, id string) error {
	// TODO: Implement Milvus delete.
	return fmt.Errorf("milvus: not implemented")
}

func (m *Milvus) Health(ctx context.Context) error {
	// TODO: Implement Milvus health check.
	return fmt.Errorf("milvus: not implemented")
}
