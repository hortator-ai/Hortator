package vectorstore

import "fmt"

// Option configures a vector store implementation.
type Option func(*options)

type options struct {
	Collection         string
	EmbeddingDimension int
}

func defaultOptions() options {
	return options{
		Collection:         "hortator-knowledge",
		EmbeddingDimension: 1536,
	}
}

// WithCollection sets the collection/index name.
func WithCollection(name string) Option {
	return func(o *options) { o.Collection = name }
}

// WithEmbeddingDimension sets the vector dimension.
func WithEmbeddingDimension(dim int) Option {
	return func(o *options) { o.EmbeddingDimension = dim }
}

// New creates a Store for the given provider.
func New(provider string, endpoint string, opts ...Option) (Store, error) {
	switch provider {
	case "qdrant":
		return NewQdrant(endpoint, opts...)
	case "milvus":
		return NewMilvus(endpoint, opts...)
	default:
		return nil, fmt.Errorf("unknown vector store provider: %s", provider)
	}
}
