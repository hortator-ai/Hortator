package vectorstore

import "context"

// Document represents an indexed piece of knowledge from a completed task.
type Document struct {
	ID        string            // task name or artifact path
	TaskID    string            // originating AgentTask name
	Namespace string
	Content   string            // text content
	Embedding []float32         // pre-computed embedding (optional, store can compute)
	Metadata  map[string]string // role, tier, tags, completedAt, etc.
}

// SearchResult is a single search hit.
type SearchResult struct {
	Document Document
	Score    float32
}

// Store is the vector store interface.
type Store interface {
	// Upsert indexes a document. Overwrites if ID exists.
	Upsert(ctx context.Context, doc Document) error

	// Search finds the top-k most similar documents.
	Search(ctx context.Context, query string, topK int, filter map[string]string) ([]SearchResult, error)

	// Delete removes a document by ID.
	Delete(ctx context.Context, id string) error

	// Health checks if the store is reachable.
	Health(ctx context.Context) error
}
