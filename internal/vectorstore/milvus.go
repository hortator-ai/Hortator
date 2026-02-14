package vectorstore

// MilvusStore is a placeholder implementation. Milvus support is not yet available.
// Contributions welcome — see https://github.com/hortator-ai/Hortator/issues

import (
	"context"
	"fmt"
)

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
	return fmt.Errorf("milvus: not yet implemented")
}

func (m *Milvus) Search(ctx context.Context, query string, topK int, filter map[string]string) ([]SearchResult, error) {
	return nil, fmt.Errorf("milvus: not yet implemented")
}

func (m *Milvus) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("milvus: not yet implemented")
}

func (m *Milvus) Health(ctx context.Context) error {
	return fmt.Errorf("milvus: not yet implemented")
}
